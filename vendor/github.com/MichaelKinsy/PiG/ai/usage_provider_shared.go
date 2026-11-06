package ai

// Ports packages/ai/src/usage/shared.ts and the shared transport pieces every
// usage fetcher needs: JSON transport over the injectable HTTP client, the
// timestamp/window/amount builders, and the provider registry.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/internal/configroot"
)

// Milliseconds in one hour.
const usageHourMs int64 = 60 * 60 * 1000

// Milliseconds in one day.
const usageDayMs int64 = 24 * usageHourMs

// Milliseconds in one seven-day week.
const usageWeekMs int64 = 7 * usageDayMs

// usageHTTPClient returns the injected client, or the process default.
func usageHTTPClient(fctx UsageFetchContext) *http.Client {
	if fctx.HTTPClient != nil {
		return fctx.HTTPClient
	}
	return http.DefaultClient
}

// usageRedirectMode selects how a probe treats redirects.
type usageRedirectMode int

const (
	// usageFollowRedirects keeps the client's own policy (the default).
	usageFollowRedirects usageRedirectMode = iota
	// usageManualRedirects returns the 3xx response to the caller.
	usageManualRedirects
	// usageRejectRedirects fails the request on any redirect.
	usageRejectRedirects
)

// usageClientFor returns the HTTP client a probe uses, applying the redirect
// policy only when no client was injected: a caller-supplied client keeps its
// own policy so tests can observe every request.
func usageClientFor(fctx UsageFetchContext, mode usageRedirectMode) *http.Client {
	client := usageHTTPClient(fctx)
	if fctx.HTTPClient != nil || mode == usageFollowRedirects {
		return client
	}
	copied := *client
	switch mode {
	case usageManualRedirects:
		copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	case usageRejectRedirects:
		copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return &copied
}

// usageResponse is one provider response, with its body already read.
type usageResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

// usageDo performs one HTTP request and reads the whole body.
func usageDo(
	ctx context.Context,
	fctx UsageFetchContext,
	mode usageRedirectMode,
	method, endpoint string,
	headers map[string]string,
	body []byte,
) (*usageResponse, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := usageClientFor(fctx, mode).Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return &usageResponse{Status: response.StatusCode, Header: response.Header, Body: payload}, nil
}

// OK reports whether the response status is 2xx.
func (response *usageResponse) OK() bool {
	return response != nil && response.Status >= 200 && response.Status < 300
}

// JSON decodes the body as a JSON value; non-JSON bodies report !ok.
func (response *usageResponse) JSON() (any, bool) {
	if response == nil || len(response.Body) == 0 {
		return nil, false
	}
	var value any
	if err := json.Unmarshal(response.Body, &value); err != nil {
		return nil, false
	}
	return value, true
}

// Text returns the body as a string.
func (response *usageResponse) Text() string {
	if response == nil {
		return ""
	}
	return string(response.Body)
}

// HeaderValue reads one response header, case-insensitively.
func (response *usageResponse) HeaderValue(name string) string {
	if response == nil {
		return ""
	}
	return response.Header.Get(name)
}

// usageStatusText is the status line tail used in auth-failure messages.
func usageStatusText(response *usageResponse) string {
	if response == nil {
		return ""
	}
	return http.StatusText(response.Status)
}

// usageHeadersLower flattens response headers into lowercased single values.
func usageHeadersLower(header http.Header) map[string]string {
	out := make(map[string]string, len(header))
	for name, values := range header {
		if len(values) == 0 {
			continue
		}
		out[strings.ToLower(name)] = values[0]
	}
	return out
}

// usageIsoTimestamp parses an ISO timestamp into epoch milliseconds.
func usageIsoTimestamp(value any) (int64, bool) {
	text, ok := value.(string)
	if !ok || text == "" {
		return 0, false
	}
	ms := DateParse(text)
	if math.IsNaN(ms) || math.IsInf(ms, 0) {
		return 0, false
	}
	return int64(ms), true
}

// usagePositiveTimestamp parses a finite positive epoch timestamp, tolerating
// seconds or milliseconds.
func usagePositiveTimestamp(value any) (int64, bool) {
	parsed, ok := usageNumber(value)
	if !ok || parsed <= 0 {
		return 0, false
	}
	if parsed < 1_000_000_000_000 {
		return int64(parsed * 1000), true
	}
	return int64(parsed), true
}

// usageMillisFromEpochSeconds converts epoch seconds to epoch milliseconds.
func usageMillisFromEpochSeconds(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0, false
	}
	return int64(value * 1000), true
}

// usageJWTClaims decodes a JWT payload without verifying its signature.
func usageJWTClaims(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, false
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	record, ok := usageRecord(payload)
	return record, ok
}

// usagePercentAmount builds a percent-unit amount from a 0..100 utilization.
func usagePercentAmount(utilization float64) UsageAmount {
	clamped := math.Min(math.Max(utilization, 0), 100)
	fraction := clamped / 100
	remaining := math.Max(0, 100-clamped)
	remainingFraction := math.Max(0, 1-fraction)
	return UsageAmount{
		Used:              &clamped,
		Limit:             new(float64(100)),
		Remaining:         &remaining,
		UsedFraction:      &fraction,
		RemainingFraction: &remainingFraction,
		Unit:              UsageUnitPercent,
	}
}

// usageFractionAmount builds a percent-unit amount from a used fraction.
func usageFractionAmount(usedFraction float64) UsageAmount {
	return usagePercentAmount(usedFraction * 100)
}

// usagePercentValue returns the used percentage of a 0..1 fraction.
func usagePercentValue(fraction float64) float64 { return fraction * 100 }

// usageWindowDurationLabel renders a window length as its label, mirroring the
// Codex "5 hours"/"7 days" formatting.
func usageWindowDurationLabel(value int64, unit string) string {
	suffix := unit
	if value != 1 {
		suffix += "s"
	}
	return strconv.FormatInt(value, 10) + " " + suffix
}

// usageWindowFromSeconds builds a window id/label pair from a span in seconds.
func usageWindowFromSeconds(seconds int64) (id string, label string) {
	if seconds >= 86_400 {
		days := int64(math.Round(float64(seconds) / 86_400))
		return strconv.FormatInt(days, 10) + "d", usageWindowDurationLabel(days, "day")
	}
	hours := seconds / 3600
	if hours < 1 {
		hours = 1
	}
	return strconv.FormatInt(hours, 10) + "h", usageWindowDurationLabel(hours, "hour")
}

// usageClampPercent clamps a 0..100 utilization value.
func usageClampPercent(value float64) float64 { return math.Min(math.Max(value, 0), 100) }

// usageIntFraction clamps a value into 0..1.
func usageIntFraction(value float64) float64 { return math.Min(math.Max(value, 0), 1) }

// usageAbsoluteURL joins a base URL and a path with exactly one slash.
func usageAbsoluteURL(base, path string) string {
	if base == "" {
		return path
	}
	if path == "" {
		return base
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

// usageHTTPURL is the part of a parsed URL a probe's endpoint derivation uses.
type usageHTTPURL struct {
	Scheme   string
	Host     string
	Hostname string
}

func (u usageHTTPURL) origin() string { return u.Scheme + "://" + u.Host }

// parseHTTPURL parses an http or https URL, reporting a failure for any other
// scheme or an unusable authority.
func parseHTTPURL(raw string) (usageHTTPURL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return usageHTTPURL{}, err
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return usageHTTPURL{}, usageErrorf("unsupported URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return usageHTTPURL{}, usageErrorf("URL %q has no host", raw)
	}
	return usageHTTPURL{Scheme: scheme, Host: parsed.Host, Hostname: parsed.Hostname()}, nil
}

// parseRequestPath returns a URL's path, or an error when unparseable.
func parseRequestPath(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return parsed.Path, nil
}

// parseFloatPrefix mirrors Number(text) for a trimmed numeric string.
func parseFloatPrefix(text string) (float64, error) {
	return strconv.ParseFloat(trimJSWhitespace(text), 64)
}

// usageOrigin returns the origin of a URL, or fallback when unparseable.
func usageOrigin(raw, fallback string) string {
	parsed, err := parseHTTPURL(raw)
	if err != nil {
		return fallback
	}
	return parsed.origin()
}

// usageTrimTrailingSlashes removes trailing path separators.
func usageTrimTrailingSlashes(value string) string { return strings.TrimRight(value, "/") }

// usageJoinNonEmpty joins parts with a separator, skipping empty parts.
func usageJoinNonEmpty(separator string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, separator)
}

// usageStringSlice converts a JSON array into its string entries.
func usageStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

// usageFormatCount renders an integer count.
func usageFormatCount(value int64) string { return strconv.FormatInt(value, 10) }

// usageFormatFloat renders a finite number without a trailing ".0".
func usageFormatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// usageErrorf formats a probe error.
func usageErrorf(format string, args ...any) error { return fmt.Errorf(format, args...) }

// usageInstallID is the stable install identity sent as `x-opencode-session`:
// the persisted install-id file when readable, else a per-process UUID. A
// missing or unwritable config root never breaks header construction.
var usageInstallID = sync.OnceValue(func() string {
	root, err := configroot.Resolve()
	if err == nil && root != "" {
		path := filepath.Join(root, "install-id")
		if data, readErr := os.ReadFile(path); readErr == nil {
			if trimmed := trimJSWhitespace(string(data)); installIDPattern.MatchString(trimmed) {
				return trimmed
			}
		}
		id, idErr := UUIDv7(nil)
		if idErr != nil {
			return randomUUIDString()
		}
		if writeErr := os.MkdirAll(root, 0o755); writeErr == nil {
			_ = os.WriteFile(path, []byte(id+"\n"), 0o600)
		}
		return id
	}
	return randomUUIDString()
})

var installIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// randomUUIDString returns a random RFC 4122 version-4 UUID.
func randomUUIDString() string {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return ""
	}
	buffer[6] = (buffer[6] & 0x0f) | 0x40
	buffer[8] = (buffer[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buffer[0:4], buffer[4:6], buffer[6:8], buffer[8:10], buffer[10:16])
}

// readKernelRelease returns the running kernel release, or "" when the
// platform does not expose one.
func readKernelRelease() string {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return trimJSWhitespace(string(data))
}

// usageRecordJSON decodes a JSON object string.
func usageRecordJSON(text string) (map[string]any, bool) {
	var payload any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil, false
	}
	return usageRecord(payload)
}

// percentEncode is encodeURIComponent.
func percentEncode(value string) string { return url.QueryEscape(value) }

// urlValues renders an application/x-www-form-urlencoded body.
func urlValues(fields map[string]string) string {
	globalKeys := make([]string, 0, len(fields))
	// Map iteration order must be stable for a reproducible request body.
	for key := range fields {
		globalKeys = append(globalKeys, key)
	}
	sortStrings(globalKeys)
	values := url.Values{}
	for _, key := range globalKeys {
		values.Set(key, fields[key])
	}
	return values.Encode()
}

// sortStrings sorts a string slice in place.
func sortStrings(values []string) { sort.Strings(values) }

// regexpMustCompile compiles a pattern that is a compile-time literal.
func regexpMustCompile(pattern string) *regexp.Regexp { return regexp.MustCompile(pattern) }

// timeFromMillis converts epoch milliseconds to a time.Time.
func timeFromMillis(millis int64) time.Time { return time.UnixMilli(millis) }

// timeNow is time.Now, kept indirect so tests can reason about window queries.
func timeNow() time.Time { return time.Now() }

// hourDuration is one hour.
const hourDuration = time.Hour

// osGetenv reads a process environment variable.
func osGetenv(name string) string { return os.Getenv(name) }

// usageUserAgent is the product User-Agent sent by usage probes.
func usageUserAgent() string { return FormatUserAgent(nil) }

// timeNowMillis returns the current epoch time in milliseconds.
func timeNowMillis() int64 { return time.Now().UnixMilli() }

// usageLogWarn records a probe warning when a logger is configured.
func usageLogWarn(fctx UsageFetchContext, message string, meta map[string]any) {
	if fctx.Logger != nil {
		fctx.Logger.Warn(message, meta)
	}
}

// usageLogDebug records a probe debug line when a logger is configured.
func usageLogDebug(fctx UsageFetchContext, message string, meta map[string]any) {
	if fctx.Logger != nil {
		fctx.Logger.Debug(message, meta)
	}
}

// usageNormalizeEmail lowercases a trimmed email address.
func usageNormalizeEmail(value string) string {
	return strings.ToLower(trimJSWhitespace(value))
}

// usageStatusFromRemaining maps a remaining fraction to a status.
func usageStatusFromRemaining(remainingFraction *float64) UsageStatus {
	if remainingFraction == nil {
		return UsageStatusUnknown
	}
	if *remainingFraction <= 0 {
		return UsageStatusExhausted
	}
	if *remainingFraction <= 0.1 {
		return UsageStatusWarning
	}
	return UsageStatusOK
}

// usageDeadline returns ctx's deadline bounded by timeout.
func usageDeadline(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, timeout)
}

// defaultUsageProviders holds the built-in providers in probe order.
var defaultUsageProviders = sync.OnceValue(func() []*UsageProvider {
	return []*UsageProvider{
		usageAlibabaProviderImpl(),
		openAICodexUsageProvider(),
		kimiUsageProvider(),
		minimaxCodeUsageProvider(),
		museCodeUsageProvider(),
		antigravityUsageProvider(),
		googleGeminiCLIUsageProvider(),
		ollamaUsageProvider(),
		factoryDroidUsageProvider(),
		ollamaCloudUsageProvider(),
		claudeUsageProvider(),
		clinePassUsageProvider(),
		zaiUsageProvider(),
		umansUsageProvider(),
		openCodeGoUsageProvider(),
		githubCopilotUsageProvider(),
		cursorUsageProvider(),
		syntheticUsageProvider(),
		xaiOAuthUsageProvider(),
		devinUsageProvider(),
		charmHyperUsageProvider(),
		commandCodeUsageProvider(),
	}
})

// DefaultUsageProviders returns the built-in usage providers in probe order.
func DefaultUsageProviders() []*UsageProvider {
	return append([]*UsageProvider(nil), defaultUsageProviders()...)
}

// DefaultUsageProvider returns the built-in usage provider for providerID.
func DefaultUsageProvider(providerID string) *UsageProvider {
	for _, provider := range defaultUsageProviders() {
		if provider.ID == providerID {
			return provider
		}
	}
	return nil
}

// usageInvalidateCacheVersion is used by callers that need to retire cached
// reports of an older provider shape.
func usageProviderCacheVersion(providerID string) int {
	if provider := DefaultUsageProvider(providerID); provider != nil {
		return provider.CacheVersion
	}
	return 0
}
