package ai

// Ports packages/ai/src/api/google-cloud-code-assist.ts and the Cloud Code Assist transport of
// @oh-my-pi/pi-ai src/providers/google-gemini-cli.ts (streamGoogleGeminiCli): the Google Antigravity and
// Gemini CLI providers' request building, SSE consumption and markup healing, in native Go.
//
// The endpoint is not the public Gemini API: a request carries a provisioned project, an account-scoped
// session id, and Antigravity expects its own tool and thinking shapes. The transport speaks
// `/v1internal:streamGenerateContent?alt=sse`, falls Antigravity back from the daily host to the sandbox
// host, and re-requests an empty stream before it surfaces it.

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/internal/nodeurl"
)

// APIGoogleGeminiCLI is the Cloud Code Assist transport API identifier: both google-antigravity and
// google-gemini-cli providers declare it, so it is the transport's identity on the wire.
//
// pig additive (D92): the value is declared here rather than in types.go so the transport family owns
// its own registration surface.
const APIGoogleGeminiCLI API = "google-gemini-cli"

// Provider identifiers served by the transport.
const (
	GoogleAntigravityProviderID = "google-antigravity"
	GoogleGeminiCliProviderID   = "google-gemini-cli"
)

// Endpoints. They are variables so a fixture server can stand in for Google in tests; production code
// never writes them.
var (
	// googleCloudCodeAssistEndpoint serves the Gemini CLI provider.
	googleCloudCodeAssistEndpoint = "https://cloudcode-pa.googleapis.com"
	// googleAntigravityPrimaryEndpoint and googleAntigravitySandboxEndpoint are the Antigravity hosts,
	// in failover order.
	googleAntigravityPrimaryEndpoint   = "https://daily-cloudcode-pa.googleapis.com"
	googleAntigravitySandboxEndpoint   = "https://daily-cloudcode-pa.sandbox.googleapis.com"
	googleCloudCodeStreamAssistPath    = "/v1internal:streamGenerateContent?alt=sse"
	googleCloudCodeAvailableModelsPath = "/v1internal:fetchAvailableModels"
)

const (
	// ccaDefaultFirstEventTimeoutMs is the wait for the first stream event; Flash models get a shorter
	// allowance because they should not inherit the cold-start budget reserved for Pro reasoning.
	ccaDefaultFirstEventTimeoutMs = 300_000
	ccaFlashFirstEventTimeoutMs   = 60_000
	// ccaStreamTimeoutMessage is the watchdog failure the stream reports.
	ccaStreamTimeoutMessage = "Cloud Code Assist stream timed out while waiting for the first event"
	// ccaMaxRetries is the number of re-attempts of a retryable response on the last endpoint.
	ccaMaxRetries = 3
	// ccaRateLimitBudgetMs caps a server-requested retry delay.
	ccaRateLimitBudgetMs = 5 * 60 * 1000
	// ccaMaxEmptyStreamRetries bounds the empty-response re-requests of one attempt.
	ccaMaxEmptyStreamRetries = 2
	// ccaEmptyStreamBaseDelayMs is the first backoff of an empty-response re-request.
	ccaEmptyStreamBaseDelayMs = 500
	// ccaClaudeThinkingBetaHeader gates Claude 4.x thinking on Antigravity.
	ccaClaudeThinkingBetaHeader = "interleaved-thinking-2025-05-14"
	// ccaRefreshSkewMs is the refresh window before a credential's expiry: inside it the next auth
	// resolution refreshes the token, while the transport still sends a token that has not yet expired.
	ccaRefreshSkewMs = 60_000
	// ccaDiscoveryTimeoutMs bounds one model discovery request.
	ccaDiscoveryTimeoutMs = 15_000
	// ccaForcedToolDirective restates a forced tool choice on Antigravity's Gemini routes, which drop
	// toolConfig (google-antigravity-forced-tool.md).
	ccaForcedToolDirective = "TOOL-ONLY TURN. This turn accepts a tool call and nothing else; a text reply here is discarded unread and you will be re-prompted. Emit the tool call now."
)

// ─── Credential blob ─────────────────────────────────────────────────────────

// ccaCredentials is the JSON credential the transport reads from the request API key. OAuth login writes
// `token` plus `projectId`; the aliases below accept a refreshed or externally produced blob.
type ccaCredentials struct {
	AccessToken  string
	ProjectID    string
	RefreshToken string
	Email        string
	AccountID    string
	// ExpiresAt is the expiry in milliseconds; nil is absent.
	ExpiresAt *float64
}

const (
	ccaInvalidCredentialsMessage = "Invalid Google Cloud Code Assist credentials. Use /login to re-authenticate."
	ccaMissingCredentialsMessage = "Missing token or projectId in Google Cloud credentials. Use /login to re-authenticate."
)

// parseCCACredentials reads the credential blob. It mirrors parseGeminiCliCredentials: a non-JSON blob and
// a missing token or project are validation failures with their own operator messages.
func parseCCACredentials(raw string) (ccaCredentials, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return ccaCredentials{}, errors.New(ccaInvalidCredentialsMessage)
	}
	if fields == nil {
		return ccaCredentials{}, errors.New(ccaInvalidCredentialsMessage)
	}
	text := func(names ...string) string {
		for _, name := range names {
			value, ok := fields[name]
			if !ok || len(value) == 0 || string(value) == "null" {
				continue
			}
			var decoded string
			if json.Unmarshal(value, &decoded) == nil {
				return decoded
			}
		}
		return ""
	}
	token := text("token")
	project := text("projectId", "project_id")
	if token == "" || project == "" {
		return ccaCredentials{}, errors.New(ccaMissingCredentialsMessage)
	}
	credentials := ccaCredentials{
		AccessToken:  token,
		ProjectID:    project,
		RefreshToken: text("refreshToken", "refresh"),
		Email:        text("email"),
		AccountID:    text("accountId"),
	}
	for _, name := range []string{"expiresAt", "expires"} {
		value, ok := fields[name]
		if !ok {
			continue
		}
		var number float64
		if json.Unmarshal(value, &number) != nil {
			continue
		}
		if expiry := normalizeCCAExpiryMs(number); expiry != nil {
			credentials.ExpiresAt = expiry
		}
	}
	return credentials, nil
}

// normalizeCCAExpiryMs accepts seconds or milliseconds, and rejects a non-positive value.
func normalizeCCAExpiryMs(value float64) *float64 {
	if value <= 0 || value != value { // NaN is never equal to itself.
		return nil
	}
	if value < 10_000_000_000 {
		value *= 1000
	}
	return &value
}

// ccaCredentialExpired reports whether the credential has already expired, which is a retry signal rather
// than something to POST: the session's auth storage refreshes it on the next attempt. A token inside the
// refresh window but not yet expired is still sent, so a stalled refresh never blocks a request.
func ccaCredentialExpired(credentials ccaCredentials, nowMs int64) bool {
	if credentials.ExpiresAt == nil {
		return false
	}
	return float64(nowMs) >= *credentials.ExpiresAt
}

// ccaCredentialNeedsRefresh is shouldRefreshGeminiCliCredentials: the credential is inside the refresh
// window, where the next resolution refreshes it.
func ccaCredentialNeedsRefresh(credentials ccaCredentials, nowMs int64) bool {
	if credentials.ExpiresAt == nil {
		return false
	}
	return float64(nowMs)+ccaRefreshSkewMs >= *credentials.ExpiresAt
}

// ccaCredentialAPIKeyJSON builds the credential blob the transport reads (login's toApiKey): the transport
// parser reads `token`, not `access`, plus `projectId`; the refresh aliases keep a refresh working.
func ccaCredentialAPIKeyJSON(access, refresh string, expiresAt *float64, projectID, email string) string {
	fields := []jsonMember{{key: "token", value: ccaMarshalJSON(access)}}
	if refresh != "" {
		fields = append(fields, jsonMember{key: "refreshToken", value: ccaMarshalJSON(refresh)})
	}
	if expiresAt != nil {
		fields = append(fields, jsonMember{key: "expiresAt", value: json.RawMessage(strconv.FormatFloat(*expiresAt, 'f', -1, 64))})
	}
	fields = append(fields, jsonMember{key: "projectId", value: ccaMarshalJSON(projectID)})
	if email != "" {
		fields = append(fields, jsonMember{key: "email", value: ccaMarshalJSON(email)})
	}
	encoded, err := jsonMembersObject(fields)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func ccaMarshalJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return encoded
}

// jsonMembersObject writes members in the order given, as JSON.stringify writes the object Pi built.
func jsonMembersObject(members []jsonMember) ([]byte, error) {
	out := make([]byte, 0, 64)
	out = append(out, '{')
	for i, member := range members {
		if i > 0 {
			out = append(out, ',')
		}
		key, err := json.Marshal(member.key)
		if err != nil {
			return nil, err
		}
		out = append(out, key...)
		out = append(out, ':')
		out = append(out, member.value...)
	}
	return append(out, '}'), nil
}

// ─── Identity: model dialect ─────────────────────────────────────────────────

// ccaIdentityClass is the request dialect a model speaks on the transport: Claude rows take the Anthropic
// shape, gpt-oss rows the Harmony shape, everything else the Gemini shape.
type ccaIdentityClass string

const (
	ccaIdentityAnthropic ccaIdentityClass = "anthropic"
	ccaIdentityOpenAI    ccaIdentityClass = "openai"
	ccaIdentityGemini    ccaIdentityClass = "gemini"
)

func ccaIdentityForModelID(id string) ccaIdentityClass {
	lower := jsstring.ToLower(id)
	switch {
	case strings.Contains(lower, "claude"):
		return ccaIdentityAnthropic
	case strings.Contains(lower, "gpt-oss"):
		return ccaIdentityOpenAI
	default:
		return ccaIdentityGemini
	}
}

// ccaSupportsFunctionPartID reports whether functionCall/functionResponse parts carry `id`: Claude through
// Cloud Code Assist does, and gpt-oss does; Gemini through this transport does not.
func ccaSupportsFunctionPartID(class ccaIdentityClass) bool {
	return class == ccaIdentityAnthropic || class == ccaIdentityOpenAI
}

// ccaRequiresFirstCallSignature reports whether an unsigned first function call must carry the bypass
// sentinel. Cloud Code Assist requires it only on that first call of a Gemini 3+ turn; secondary parallel
// calls may stay unsigned.
func ccaRequiresFirstCallSignature(class ccaIdentityClass, modelID string) bool {
	if class != ccaIdentityGemini {
		return false
	}
	major, ok := getGeminiMajorVersion(modelID)
	return ok && major >= 3
}

// ccaClaudeRevision reads the Claude family revision (`claude-sonnet-4-6` is 4.6); ok is false for an id
// without one.
func ccaClaudeRevision(id string) (float64, bool) {
	lower := jsstring.ToLower(id)
	index := strings.Index(lower, "claude")
	if index < 0 {
		return 0, false
	}
	digits := func(from int) (int, int) {
		for from < len(lower) && (lower[from] < '0' || lower[from] > '9') {
			from++
			// The family name ends at the next number run; skip a single family token only.
			if from < len(lower) && lower[from] == '-' && from+1 < len(lower) && (lower[from+1] < '0' || lower[from+1] > '9') {
				continue
			}
			break
		}
		start := from
		for from < len(lower) && lower[from] >= '0' && lower[from] <= '9' {
			from++
		}
		return start, from
	}
	start, end := digits(index + len("claude"))
	if end <= start {
		return 0, false
	}
	major, err := strconv.Atoi(lower[start:end])
	if err != nil {
		return 0, false
	}
	revision := float64(major)
	if end < len(lower) && lower[end] == '-' {
		minorStart, minorEnd := digits(end + 1)
		if minorEnd > minorStart {
			if minor, err := strconv.Atoi(lower[minorStart:minorEnd]); err == nil {
				revision += float64(minor) / 10
			}
		}
	}
	return revision, true
}

// ccaUsesClaudeThinkingBeta reports whether the request adds the interleaved-thinking beta header: Claude
// 4.x on Antigravity, whose thinking streams only with it. The native 2.26.0 client sends no beta for
// Claude 5.x.
func ccaUsesClaudeThinkingBeta(class ccaIdentityClass, isAntigravity, reasoning bool, modelID string) bool {
	if !isAntigravity || !reasoning || class != ccaIdentityAnthropic {
		return false
	}
	revision, ok := ccaClaudeRevision(modelID)
	return !ok || revision < 5
}

// ─── Client identity headers ─────────────────────────────────────────────────

const (
	ccaDefaultGeminiCliVersion  = "0.46.0"
	ccaDefaultAntigravityVer    = "2.26.0"
	ccaDefaultAntigravityChange = "1005016104"
	// ccaAntigravityManifestURL is the official electron-builder update manifest of the client.
	ccaAntigravityManifestURL = "https://antigravity-hub-auto-updater-974169037036.us-central1.run.app/manifest/latest-arm64-mac.yml"
	// ccaAntigravityManifestTimeoutMs bounds the version lookup.
	ccaAntigravityManifestTimeoutMs = 5_000
	// ccaAntigravityManifestRetryMs suppresses repeated failed lookups.
	ccaAntigravityManifestRetryMs = 10 * 60_000
)

// ccaGeminiCliUserAgent is the Gemini CLI client string that unlocks the CLI's rate limits:
// GeminiCLI/VERSION/MODEL (PLATFORM; ARCH; SURFACE).
func ccaGeminiCliUserAgent(modelID string) string {
	version := os.Getenv("PI_AI_GEMINI_CLI_VERSION")
	if version == "" {
		version = ccaDefaultGeminiCliVersion
	}
	if modelID == "" {
		modelID = "gemini-3.1-pro-preview"
	}
	return "GeminiCLI/" + version + "/" + modelID + " (" + ccaNodePlatform() + "; " + ccaNodeArch() + "; terminal)"
}

// ccaGeminiCliHeaders are the Gemini CLI provider's client headers.
func ccaGeminiCliHeaders(modelID string) map[string]string {
	return map[string]string{
		"User-Agent":      ccaGeminiCliUserAgent(modelID),
		"Client-Metadata": "ideType=IDE_UNSPECIFIED,platform=PLATFORM_UNSPECIFIED,pluginType=GEMINI",
	}
}

func ccaNodePlatform() string {
	if runtime.GOOS == "windows" {
		return "win32"
	}
	return runtime.GOOS
}

func ccaNodeArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "ia32"
	default:
		return runtime.GOARCH
	}
}

// ccaAntigravityPlatform is the native build the host would run. OSes without a build report the Linux
// client, the closest one that exists.
func ccaAntigravityPlatform() (osType, arch string) {
	switch runtime.GOOS {
	case "darwin":
		return "darwin", "arm64"
	case "linux":
		return "linux", "amd64"
	case "windows":
		return "windows", "amd64"
	default:
		return "linux", "amd64"
	}
}

var ccaAntigravityVersionState struct {
	mu         sync.Mutex
	discovered string
	fetching   *sync.WaitGroup
	failedAt   time.Time
	failed     bool
}

// ccaAntigravityVersion is the client version: env override, manifest discovery, then the pinned fallback.
func ccaAntigravityVersion() string {
	if version := os.Getenv("PI_AI_ANTIGRAVITY_VERSION"); version != "" {
		return version
	}
	ccaAntigravityVersionState.mu.Lock()
	defer ccaAntigravityVersionState.mu.Unlock()
	if ccaAntigravityVersionState.discovered != "" {
		return ccaAntigravityVersionState.discovered
	}
	return ccaDefaultAntigravityVer
}

// ccaEnsureAntigravityVersion resolves the latest client release once per process. Failures are silent: the
// pinned fallback stays valid, and further lookups are suppressed for a while so a request never stalls.
func ccaEnsureAntigravityVersion(ctx context.Context, fetch *http.Client) {
	if os.Getenv("PI_AI_ANTIGRAVITY_VERSION") != "" {
		return
	}
	ccaAntigravityVersionState.mu.Lock()
	if ccaAntigravityVersionState.discovered != "" {
		ccaAntigravityVersionState.mu.Unlock()
		return
	}
	if ccaAntigravityVersionState.failed && time.Since(ccaAntigravityVersionState.failedAt) < ccaAntigravityManifestRetryMs*time.Millisecond {
		ccaAntigravityVersionState.mu.Unlock()
		return
	}
	if ccaAntigravityVersionState.fetching != nil {
		wait := ccaAntigravityVersionState.fetching
		ccaAntigravityVersionState.mu.Unlock()
		wait.Wait()
		return
	}
	wait := &sync.WaitGroup{}
	wait.Add(1)
	ccaAntigravityVersionState.fetching = wait
	ccaAntigravityVersionState.mu.Unlock()

	found := ccaDiscoverAntigravityVersion(ctx, fetch)

	ccaAntigravityVersionState.mu.Lock()
	ccaAntigravityVersionState.fetching = nil
	if found != "" {
		ccaAntigravityVersionState.discovered = found
		ccaAntigravityVersionState.failed = false
	} else {
		ccaAntigravityVersionState.failed = true
		ccaAntigravityVersionState.failedAt = time.Now()
	}
	ccaAntigravityVersionState.mu.Unlock()
	wait.Done()
}

// ccaDiscoverAntigravityVersion reads the client version from the update manifest.
func ccaDiscoverAntigravityVersion(ctx context.Context, fetch *http.Client) string {
	client := fetch
	if client == nil {
		client = http.DefaultClient
	}
	requestCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ccaAntigravityManifestTimeoutMs*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, ccaAntigravityManifestURL, nil)
	if err != nil {
		return ""
	}
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("User-Agent", "electron-builder")
	response, err := client.Do(request)
	if err != nil {
		return ""
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<16))
	if err != nil {
		return ""
	}
	return ccaParseAntigravityManifestVersion(string(body))
}

// ccaParseAntigravityManifestVersion extracts `version: x.y.z` from the manifest.
func ccaParseAntigravityManifestVersion(yamlText string) string {
	for _, line := range strings.Split(yamlText, "\n") {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "version") {
			continue
		}
		rest, ok := strings.CutPrefix(trimmed, "version")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, ":") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(rest, ":"))
		if cut, _, ok := strings.Cut(value, "#"); ok {
			value = strings.TrimSpace(cut)
		}
		value = strings.Trim(value, `"'`)
		if ccaIsDottedVersion(value) {
			return value
		}
	}
	return ""
}

func ccaIsDottedVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return true
}

// ccaAntigravityUserAgent is the Antigravity client string. The backend does not validate `cl`; only the
// version gates newer models.
func ccaAntigravityUserAgent() string {
	version := ccaAntigravityVersion()
	change := cmp.Or(os.Getenv("PI_AI_ANTIGRAVITY_CL"), ccaDefaultAntigravityChange)
	osType, arch := ccaAntigravityPlatform()
	osType = cmp.Or(os.Getenv("PI_AI_ANTIGRAVITY_OS"), osType)
	arch = cmp.Or(os.Getenv("PI_AI_ANTIGRAVITY_ARCH"), arch)
	return "antigravity/hub/" + version + " (aidev_client; os_type=" + osType + "; arch=" + arch + "; cl=" + change + ")"
}

// ccaAntigravityHeaders are the native client's headers: Go net/http offers gzip and no Accept.
func ccaAntigravityHeaders() map[string]string {
	return map[string]string{
		"User-Agent":      ccaAntigravityUserAgent(),
		"Accept-Encoding": "gzip",
	}
}

// ─── Transport bootstrap ─────────────────────────────────────────────────────

// GoogleCloudCodeAssistAPI is the Cloud Code Assist API implementation stream pair.
func GoogleCloudCodeAssistAPI() *ProviderStreams { return apiStreams(APIGoogleGeminiCLI) }

// GoogleCloudCodeAssistConfig configures a Cloud Code Assist provider for one model.
type GoogleCloudCodeAssistConfig struct {
	APIKey        string
	Model         string
	ProviderID    string
	BaseURL       string
	ExtraHeaders  map[string]string
	ModelMetadata *Model
}

// NewGoogleCloudCodeAssistProvider creates the transport provider for cfg.
func NewGoogleCloudCodeAssistProvider(cfg GoogleCloudCodeAssistConfig) Provider {
	if cfg.ProviderID == "" {
		cfg.ProviderID = GoogleGeminiCliProviderID
	}
	return &googleCloudCodeAssistProvider{cfg: cfg, client: streamingHTTPClientNoRetry()}
}

// GoogleCloudCodeAssistProviderFromConfig is NewGoogleCloudCodeAssistProvider for the API factory.
func GoogleCloudCodeAssistProviderFromConfig(cfg GoogleCloudCodeAssistConfig) (Provider, error) {
	return NewGoogleCloudCodeAssistProvider(cfg), nil
}

type googleCloudCodeAssistProvider struct {
	cfg    GoogleCloudCodeAssistConfig
	client *http.Client
}

func (p *googleCloudCodeAssistProvider) ID() string { return p.cfg.ProviderID }
func (p *googleCloudCodeAssistProvider) Close() error {
	p.client.CloseIdleConnections()
	return nil
}

// ccaRequestModel is the model the request builds from: the selected metadata when given, else the bundled
// catalog entry of the provider/model, else the configured identity alone.
func (p *googleCloudCodeAssistProvider) requestModel() *Model {
	if p.cfg.ModelMetadata != nil {
		selected := *p.cfg.ModelMetadata
		return &selected
	}
	if generated, ok := LookupModel(p.cfg.ProviderID + "/" + p.cfg.Model); ok {
		return generated.ToModel()
	}
	if generated, ok := LookupModel(p.cfg.Model); ok {
		return generated.ToModel()
	}
	return &Model{ID: p.cfg.Model, Capabilities: ModelCapabilities{MaxThinking: ThinkingLevelHigh}, ProviderMeta: ProviderMetadata{API: APIGoogleGeminiCLI, ProviderID: p.cfg.ProviderID, BaseURL: p.cfg.BaseURL}}
}

// ccaRequestURL joins an endpoint and the stream path.
func ccaRequestURL(endpoint string) (string, error) {
	return nodeurl.ResolveDirectoryPath(endpoint, strings.TrimPrefix(googleCloudCodeStreamAssistPath, "/"))
}

// ccaApplyHeaders sets the request headers in the source's order: auth and content type, then the client
// headers of the provider, then the Claude beta, then the request's own headers (which win).
func ccaApplyHeaders(request *http.Request, accessToken string, clientHeaders map[string]string, claudeBeta bool, extra map[string]string, providerHeaders ProviderHeaders) {
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	for name, value := range clientHeaders {
		request.Header.Set(name, value)
	}
	if claudeBeta {
		request.Header.Set("anthropic-beta", ccaClaudeThinkingBetaHeader)
	}
	for name, value := range extra {
		request.Header.Set(name, value)
	}
	applyProviderHeaders(request, providerHeaders)
}

// ccaHTTPError is a status rejection of the endpoint. It exposes the status and headers so the shared retry
// policy can classify it, and composes the message the transport reports.
type ccaHTTPError struct {
	message string
	status  int
	headers http.Header
}

func (e *ccaHTTPError) Error() string                { return e.message }
func (e *ccaHTTPError) ProviderStatus() *int         { return &e.status }
func (e *ccaHTTPError) ProviderHeaders() http.Header { return e.headers }

// ccaAPIError is an in-band stream failure carrying a status, which the failover decision classifies.
type ccaAPIError struct {
	message string
	status  int
}

func (e *ccaAPIError) Error() string        { return e.message }
func (e *ccaAPIError) ProviderStatus() *int { return &e.status }

// ccaEmptyOutputError marks a response the transport must not replay onto another endpoint: the message
// itself completed a reasoning pass, so a failover would burn another one.
type ccaEmptyOutputError struct{ message string }

func (e *ccaEmptyOutputError) Error() string { return e.message }

// ccaStatusOf reads the status an error carries, or 0.
func ccaStatusOf(err error) int {
	type statusError interface {
		ProviderStatus() *int
	}
	var reported statusError
	if errors.As(err, &reported) {
		if status := reported.ProviderStatus(); status != nil {
			return *status
		}
	}
	return 0
}

// ccaTransientStatus is AIError.isTransientStatus: a rate limit, a timeout, a conflict or a server fault.
func ccaTransientStatus(status int) bool {
	switch {
	case status == 0:
		return false
	case status == http.StatusRequestTimeout, status == http.StatusConflict, status == http.StatusTooManyRequests:
		return true
	}
	return status >= 500 && status <= 599
}

// ccaReadErrorBody reads a bounded error body as text.
func ccaReadErrorBody(body io.Reader) string {
	if body == nil {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(body, 64*1024))
	if err != nil && len(data) == 0 {
		return ""
	}
	return fetchBodyText(data)
}

// ccaValidationURL extracts a VALIDATION_REQUIRED detail's validation_url from an error body.
func ccaValidationURL(errorBody string) string {
	if !strings.Contains(errorBody, "VALIDATION_REQUIRED") {
		return ""
	}
	start := strings.Index(errorBody, "{")
	if start < 0 {
		return ""
	}
	var parsed struct {
		Error struct {
			Details []struct {
				Reason   string `json:"reason"`
				Metadata struct {
					ValidationURL string `json:"validation_url"`
				} `json:"metadata"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(errorBody[start:]), &parsed) != nil {
		return ""
	}
	for _, detail := range parsed.Error.Details {
		if detail.Reason == "VALIDATION_REQUIRED" && detail.Metadata.ValidationURL != "" {
			return detail.Metadata.ValidationURL
		}
	}
	return ""
}

// ccaValidationMessage is formatGoogleValidationRequiredMessage.
func ccaValidationMessage(validationURL, nextAction, email string) string {
	account := ""
	if email != "" {
		account = " for " + email
	}
	return "Account verification required" + account + ". Visit " + validationURL + " to continue, then " + nextAction + "."
}

// ccaErrorText is the message of a non-2xx response: the validation instruction when Google asked for one,
// else the body.
func ccaErrorText(status int, body, email string) string {
	if validationURL := ccaValidationURL(body); validationURL != "" {
		return ccaValidationMessage(validationURL, "retry your request", email)
	}
	if trimmed := strings.TrimSpace(body); trimmed != "" {
		return trimmed
	}
	return fmt.Sprintf("Request failed with status %d", status)
}

// ─── Stream entry points ─────────────────────────────────────────────────────

// Stream streams one Cloud Code Assist request. The credential is the OAuth blob in the API key; Cloud Code
// has no API-key mode.
func (p *googleCloudCodeAssistProvider) Stream(ctx context.Context, transcript TranscriptContext, options StreamOptions) (*AssistantMessageEventStream, error) {
	// pig divergence (D48): the agent loop asks for the tool-flow repair.
	if options.RepairToolFlow {
		transcript = transcript.withRepairedToolFlow()
	}
	if err := validateProviderRequest(ctx, transcript); err != nil {
		return nil, fmt.Errorf("google cloud code: invalid transcript: %w", err)
	}
	apiKey := cmp.Or(options.APIKey, p.cfg.APIKey)
	if apiKey == "" {
		return nil, errors.New("Google Cloud Code Assist requires OAuth authentication. Use /login to authenticate.")
	}
	credentials, err := parseCCACredentials(apiKey)
	if err != nil {
		return nil, err
	}
	if ccaCredentialExpired(credentials, nowMillis()) {
		return nil, errors.New("OAuth token expired before request — please retry; AuthStorage will refresh on the next attempt.")
	}
	model := p.requestModel()
	isAntigravity := p.cfg.ProviderID == GoogleAntigravityProviderID
	class := ccaIdentityForModelID(model.ID)
	ccaOptions := googleCloudCodeOptionsOf(options)
	// A host that declares an image contract (Claude behind Antigravity) rejects oversized images, and a
	// rejected image stays in context, so clamp before building.
	transcript = ccaClampImageContext(model, transcript, class, isAntigravity)
	endpoints, err := ccaSelectEndpoints(cmp.Or(p.cfg.BaseURL, model.ProviderMeta.BaseURL), isAntigravity, ccaOptions)
	if err != nil {
		return nil, err
	}
	accountKey := ""
	if isAntigravity {
		accountKey = ccaAccountKeyOf(credentials)
	}
	wireModelID, reviewed := ccaRouteWireID(accountKey, model.ID, ccaOptions, ccaThinkingLevelOfRequest(options))
	projectID := cmp.Or(optionString(ccaOptions, func(o *GoogleCloudCodeOptions) string { return o.ProjectID }), credentials.ProjectID)
	body, err := ccaBuildRequestBody(ctx, model, transcript, ccaRequestPlan{
		providerID: p.cfg.ProviderID, projectID: projectID, accountKey: accountKey,
		wireModelID: wireModelID, reviewed: reviewed, isAntigravity: isAntigravity, class: class,
		options: options, ccaOptions: ccaOptions,
	})
	if err != nil {
		return nil, err
	}
	var payload any = body
	if options.OnPayload != nil {
		replacement, err := options.OnPayload(body, model)
		if err != nil {
			return nil, fmt.Errorf("google cloud code: onPayload: %w", err)
		}
		if replacement != nil {
			payload = payloadReplacement(replacement)
		}
	}
	encoded, err := marshalRequestBody(payload)
	if err != nil {
		return nil, fmt.Errorf("google cloud code: marshal request: %w", err)
	}
	if ccaOptions != nil && len(ccaOptions.ExtraBody) > 0 {
		encoded, err = ccaApplyExtraBody(encoded, ccaOptions.ExtraBody)
		if err != nil {
			return nil, fmt.Errorf("google cloud code: extra body: %w", err)
		}
	}
	if isAntigravity {
		ccaEnsureAntigravityVersion(ctx, options.Fetch)
	}
	clientHeaders := ccaGeminiCliHeaders(model.ID)
	if isAntigravity {
		clientHeaders = ccaAntigravityHeaders()
	} else {
		clientHeaders["Accept"] = "text/event-stream"
	}
	claudeBeta := ccaUsesClaudeThinkingBeta(class, isAntigravity, model.ProviderMeta.Reasoning || options.IsReasoning, model.ID)
	stream := newCCAStream(ctx, model, p.cfg.ProviderID, credentials, class, options, ccaOptions, transcript, encoded, endpoints, clientHeaders, claudeBeta, p.client)
	return stream.builder.stream, nil
}

// optionString reads one provider option, tolerating a nil options value.
func optionString(options *GoogleCloudCodeOptions, read func(*GoogleCloudCodeOptions) string) string {
	if options == nil {
		return ""
	}
	return read(options)
}

func trimSpaceJS(value string) string { return jsstring.Trim(value) }

// ─── Stream state ────────────────────────────────────────────────────────────

// ccaStream owns one request's attempt loop, its event builder and the response folding state that has to
// be reset between empty-response re-requests.
type ccaStream struct {
	builder       *assistantStreamBuilder
	ctx           context.Context
	model         *Model
	providerID    string
	credentials   ccaCredentials
	class         ccaIdentityClass
	isAntigravity bool
	options       StreamOptions
	ccaOptions    *GoogleCloudCodeOptions
	session       *CCAAntigravitySessionState
	eventModel    *Model
	body          []byte
	endpoints     []string
	clientHeaders map[string]string
	claudeBeta    bool
	client        *http.Client

	usage           Usage
	finishReason    string
	sawFinishReason bool
	receivedContent bool
	errorMessage    string
	lastResponseID  string

	// Response folding state, reset per attempt.
	activeText     int
	activeThinking int
	healing        *ccaThinkingHealer
	buffering      bool
	textBuffer     string
	bufferSig      string
	strippedLeak   bool
	toolNames      map[string]struct{}
	flashLeak      bool
}

func newCCAStream(ctx context.Context, model *Model, providerID string, credentials ccaCredentials, class ccaIdentityClass, options StreamOptions, ccaOptions *GoogleCloudCodeOptions, transcript TranscriptContext, body []byte, endpoints []string, clientHeaders map[string]string, claudeBeta bool, client *http.Client) *ccaStream {
	stream := &ccaStream{
		ctx: ctx, model: model, providerID: providerID, credentials: credentials, class: class,
		options: options, body: body, endpoints: endpoints, clientHeaders: clientHeaders, claudeBeta: claudeBeta,
		client: client, isAntigravity: providerID == GoogleAntigravityProviderID,
		ccaOptions: ccaOptions, session: ccaSessionOf(ctx, ccaOptions),
		activeText: -1, activeThinking: -1,
	}
	stream.builder = newObservedProviderBuilder(ctx, APIGoogleGeminiCLI, providerID, model.ID)
	stream.builder.modelCost = options.ModelCost
	stream.eventModel = providerEventModel(model, APIGoogleGeminiCLI, providerID, model.ID)
	stream.builder.setProviderEventObserver(options, stream.eventModel)
	stream.toolNames = ccaRequestToolNames(transcript)
	stream.flashLeak = ccaFlashStreamLeakWorkaround(providerID, model.ID)
	go stream.run()
	return stream
}

// ccaFlashStreamLeakWorkaround is the flash family's duplicate-first-chunk workaround on Cloud Code Assist.
func ccaFlashStreamLeakWorkaround(providerID, modelID string) bool {
	if providerID != GoogleAntigravityProviderID && providerID != GoogleGeminiCliProviderID {
		return false
	}
	class := ccaIdentityForModelID(modelID)
	if class != ccaIdentityGemini {
		return false
	}
	lower := jsstring.ToLower(modelID)
	return strings.Contains(lower, "gemini") && strings.Contains(lower, "flash")
}

// ccaRequestToolNames is the tool roster of the request, which the planning-leak filter matches against.
func ccaRequestToolNames(transcript TranscriptContext) map[string]struct{} {
	names := map[string]struct{}{}
	for _, tool := range borrowedCurrentTools(collapsedMessages(transcript)) {
		names[tool.Name] = struct{}{}
	}
	return names
}

// startTextOrThinkingBlock opens the requested block kind and closes the other open one, as
// google-shared.ts startTextOrThinkingBlock does.
func (state *ccaStream) startTextBlock() {
	if state.activeText >= 0 {
		return
	}
	state.endThinkingBlock()
	state.activeText = len(state.builder.partial.Content)
	state.builder.partial.Content = append(state.builder.partial.Content, TextContent{})
	state.builder.start()
	state.builder.push(TextStartEvent{ContentIndex: state.activeText, Partial: state.builder.partial})
}

func (state *ccaStream) startThinkingBlock() {
	if state.activeThinking >= 0 {
		return
	}
	state.endTextBlock()
	state.activeThinking = len(state.builder.partial.Content)
	state.builder.partial.Content = append(state.builder.partial.Content, ThinkingContent{})
	state.builder.start()
	state.builder.push(ThinkingStartEvent{ContentIndex: state.activeThinking, Partial: state.builder.partial})
}

// endTextBlock closes the open text block with its end event.
func (state *ccaStream) endTextBlock() {
	if state.activeText < 0 {
		return
	}
	index := state.activeText
	block := state.builder.partial.Content[index].(TextContent)
	state.activeText = -1
	state.builder.push(TextEndEvent{ContentIndex: index, Content: block.Text, Partial: state.builder.partial})
}

// endThinkingBlock closes the open thinking block with its end event.
func (state *ccaStream) endThinkingBlock() {
	if state.activeThinking < 0 {
		return
	}
	index := state.activeThinking
	block := state.builder.partial.Content[index].(ThinkingContent)
	state.activeThinking = -1
	state.builder.push(ThinkingEndEvent{ContentIndex: index, Content: block.Thinking, Partial: state.builder.partial})
}

// endCurrentBlock closes the open text or thinking block with its end event.
func (state *ccaStream) endCurrentBlock() {
	state.endTextBlock()
	state.endThinkingBlock()
}

// emitVisibleText appends visible text to the open text block.
func (state *ccaStream) emitVisibleText(delta, signature string) {
	if delta == "" {
		return
	}
	state.startTextBlock()
	index := state.activeText
	block := state.builder.partial.Content[index].(TextContent)
	block.Text += delta
	block.TextSignature = RetainThoughtSignature(block.TextSignature, signature)
	state.builder.partial.Content[index] = block
	state.builder.push(TextDeltaEvent{ContentIndex: index, Delta: delta, Partial: state.builder.partial})
}

// emitVisibleThinking appends a thinking delta to the open thinking block.
func (state *ccaStream) emitVisibleThinking(delta string) {
	if delta == "" {
		return
	}
	state.startThinkingBlock()
	index := state.activeThinking
	block := state.builder.partial.Content[index].(ThinkingContent)
	block.Thinking += delta
	state.builder.partial.Content[index] = block
	state.builder.push(ThinkingDeltaEvent{ContentIndex: index, Delta: delta, Partial: state.builder.partial})
}

// retainCurrentBlockSignature keeps a signature-only part's signature on the open block.
func (state *ccaStream) retainCurrentBlockSignature(signature string) {
	if signature == "" {
		return
	}
	if state.activeThinking >= 0 {
		index := state.activeThinking
		block := state.builder.partial.Content[index].(ThinkingContent)
		block.ThinkingSignature = RetainThoughtSignature(block.ThinkingSignature, signature)
		state.builder.partial.Content[index] = block
		return
	}
	if state.activeText >= 0 {
		index := state.activeText
		block := state.builder.partial.Content[index].(TextContent)
		block.TextSignature = RetainThoughtSignature(block.TextSignature, signature)
		state.builder.partial.Content[index] = block
	}
}

// feedVisibleText heals leaked thinking markup out of the visible channel.
func (state *ccaStream) feedVisibleText(delta, signature string) {
	if state.healing == nil {
		state.healing = newCCAThinkingHealer()
	}
	for _, event := range state.healing.feed(delta) {
		switch event.kind {
		case ccaHealText:
			state.emitVisibleText(event.text, signature)
		case ccaHealThinkingDelta:
			state.emitVisibleThinking(event.text)
		}
	}
}

// flushVisibleText releases held-back healing markup.
func (state *ccaStream) flushVisibleText(signature string) {
	if state.healing == nil {
		return
	}
	for _, event := range state.healing.flush() {
		switch event.kind {
		case ccaHealText:
			state.emitVisibleText(event.text, signature)
		case ccaHealThinkingDelta:
			state.emitVisibleThinking(event.text)
		}
	}
}

// resetAttempt clears the message and folding state between empty-response re-requests. Events already
// pushed keep the partials that described them.
func (state *ccaStream) resetAttempt() {
	state.endCurrentBlock()
	state.builder.partial.Content = nil
	state.builder.toolCalls = map[int]*streamToolCall{}
	state.builder.partial.Usage = Usage{}
	state.builder.partial.StopReason = StopReasonStop
	state.builder.partial.ErrorMessage = ""
	state.builder.partial.Timestamp = nowMillis()
	state.activeText, state.activeThinking = -1, -1
	state.usage = Usage{}
	state.finishReason = ""
	state.sawFinishReason = false
	state.receivedContent = false
	state.errorMessage = ""
	state.lastResponseID = ""
	state.healing = nil
	state.buffering = false
	state.textBuffer = ""
	state.bufferSig = ""
	state.strippedLeak = false
}

// handledToolCall records a function call as a fully built tool call and pushes its lifecycle events.
func (state *ccaStream) handledToolCall(index int, call ToolCall) {
	state.builder.toolCallDelta(streamToolCallDelta{
		index: index, id: call.ID, name: call.Name, argumentsDelta: string(call.argumentsJSONOrEmpty()), initialArguments: call.Arguments, thoughtSignature: call.ThoughtSignature, recordOnly: true,
	})
	state.builder.endToolCall(index)
}

// argumentsJSONOrEmpty is the JSON text of a call's arguments, `{}` when it has none.
func (call ToolCall) argumentsJSONOrEmpty() json.RawMessage {
	encoded, err := call.ArgumentsJSON()
	if err != nil || len(encoded) == 0 {
		return json.RawMessage(`{}`)
	}
	return encoded
}

// ccaHasMeaningfulContent is hasMeaningfulGoogleContent: a tool call or non-whitespace text.
func ccaHasMeaningfulContent(content []AssistantContentBlock) bool {
	for _, block := range content {
		switch block := block.(type) {
		case ToolCall:
			return true
		case TextContent:
			if trimJSWhitespace(block.Text) != "" {
				return true
			}
		}
	}
	return false
}

// ccaHasThinkingOutput reports a message whose only content is thinking worth surfacing.
func ccaHasThinkingOutput(content []AssistantContentBlock) bool {
	for _, block := range content {
		if thinking, ok := block.(ThinkingContent); ok && (trimJSWhitespace(thinking.Thinking) != "" || thinking.ThinkingSignature != "") {
			return true
		}
	}
	return false
}

// ccaMapUsage is google-shared.ts mapGoogleUsage: promptTokenCount includes cached content, the prompt falls
// back to total-candidates-thoughts, and cached tokens are clamped to it so input is never negative.
func ccaMapUsage(metadata *ccaUsageMetadata) Usage {
	candidates := metadata.CandidatesTokenCount
	thinking := metadata.ThoughtsTokenCount
	total := metadata.TotalTokenCount
	prompt := metadata.PromptTokenCount
	if prompt == 0 {
		prompt = max(0, total-candidates-thinking)
	}
	cacheRead := min(metadata.CachedContentTokenCount, prompt)
	usage := Usage{
		Input:       prompt - cacheRead,
		Output:      candidates + thinking,
		CacheRead:   cacheRead,
		TotalTokens: total,
	}
	if thinking > 0 {
		reasoning := thinking
		usage.Reasoning = &reasoning
	}
	return usage
}

// ccaStreamToolCounter numbers generated tool-call ids of the transport.
var ccaStreamToolCounter atomic.Int64
