package ai

// Cloud Code Assist request options, per-account wire-profile isolation, Antigravity effort-tier collapsing
// and the Claude image clamp. These are the provider-specific knobs ported from OMP's
// GoogleGeminiCliOptions / AntigravityProviderSessionState and the google-antigravity class rules.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/jpeg"
	"image/png"
	"strconv"
	"strings"
	"sync"
)

// RawSSEEvent is one server-sent event as it arrived, before any parsing.
type RawSSEEvent struct {
	Event string
	Data  string
	Raw   []string
}

// CCAAntigravitySessionState is the per-conversation Antigravity request identity and endpoint memory that
// mirrors the real client. It is safe for use by one conversation at a time, which is how a session uses it.
type CCAAntigravitySessionState struct {
	// AgentID and TrajectoryID are the envelope UUIDs; AgentID doubles as labels.cascade_id.
	AgentID string
	// TrajectoryID is labels.trajectory_id.
	TrajectoryID string
	// SessionID is the signed-decimal request sessionId.
	SessionID string
	// StepIndex is the last requestId step (monotonic); RequestCount counts envelopes of the trajectory.
	StepIndex    int
	RequestCount int
	// LastExecutionID is the prior response id echoed as labels.last_execution_id.
	LastExecutionID string
	// LastGoodEndpoint is the Antigravity host that last answered, tried first in auto mode.
	LastGoodEndpoint string
}

// GoogleCloudCodeOptions are the Cloud Code Assist transport's provider-specific request options. Pi's
// GoogleGeminiCliOptions carries them as top-level members; they live in one struct here so the shared
// StreamOptions surface gains a single field.
type GoogleCloudCodeOptions struct {
	// HideThinkingSummary asks Cloud Code Assist to omit human-readable thought summaries while still
	// allowing internal reasoning.
	HideThinkingSummary bool
	// RequestModelID overrides the wire model id (collapsed effort-tier variants name their routed SKU).
	RequestModelID string
	// ProjectID overrides the provisioned project of the credential blob.
	ProjectID string
	// EndpointMode is Antigravity routing: "auto" (default, production then sandbox), "production", or
	// "sandbox".
	EndpointMode string
	// AcceptEmptyResponse accepts an eventless or blank completion as the final answer instead of surfacing
	// an empty-response failure.
	AcceptEmptyResponse bool
	// FirstEventTimeoutMs overrides the first-event watchdog; zero means the provider default.
	FirstEventTimeoutMs *int
	// OnSSEEvent observes every server-sent event before it is normalized, for diagnostics.
	OnSSEEvent func(ctx context.Context, event RawSSEEvent, model *Model) error
	// Session carries the conversation's Antigravity envelope identity across requests.
	Session *CCAAntigravitySessionState
	// ExtraBody merges additional members into the request object, after the built ones.
	ExtraBody map[string]any
	// UsageLabel overrides the Antigravity `used_claude` usage bucket. It is ALWAYS written as a JSON
	// string: an object there makes Cloud Code Assist reject the request with a 403 license error.
	UsageLabel *string
}

// googleCloudCodeOptionsOf reads the transport options of a request. A caller that cannot set the typed
// field can pass the same shape under StreamOptions.Metadata["googleCloudCode"].
func googleCloudCodeOptionsOf(options StreamOptions) *GoogleCloudCodeOptions {
	if options.GoogleCloudCode != nil {
		return options.GoogleCloudCode
	}
	if raw, ok := options.Metadata["googleCloudCode"]; ok {
		if typed, ok := raw.(*GoogleCloudCodeOptions); ok {
			return typed
		}
		if value, ok := raw.(GoogleCloudCodeOptions); ok {
			return &value
		}
	}
	return nil
}

// CCAAntigravityEndpoints reports the Antigravity hosts in failover order, as the transport currently
// resolves them.
func CCAAntigravityEndpoints() (primary, sandbox string) {
	return googleAntigravityPrimaryEndpoint, googleAntigravitySandboxEndpoint
}

// ccaEndpointMode is the request's Antigravity routing mode, defaulting to auto.
func ccaEndpointMode(options *GoogleCloudCodeOptions) string {
	if options == nil {
		return "auto"
	}
	switch options.EndpointMode {
	case "production", "sandbox":
		return options.EndpointMode
	default:
		return "auto"
	}
}

// ccaSelectEndpoints is the source's endpoint list for one request. The mode pins a host; auto prefers the
// conversation's last good host and otherwise fails over production → sandbox. A base URL outside the two
// known hosts is used alone.
func ccaSelectEndpoints(baseURL string, isAntigravity bool, options *GoogleCloudCodeOptions) ([]string, error) {
	trimmed := strings.TrimRight(trimSpaceJS(baseURL), "/")
	if !isAntigravity {
		if trimmed != "" {
			return []string{trimmed}, nil
		}
		return []string{googleCloudCodeAssistEndpoint}, nil
	}
	production := strings.TrimRight(googleAntigravityPrimaryEndpoint, "/")
	sandbox := strings.TrimRight(googleAntigravitySandboxEndpoint, "/")
	mode := ccaEndpointMode(options)
	known := trimmed == "" || trimmed == production || trimmed == sandbox
	if !known {
		return []string{trimmed}, nil
	}
	switch mode {
	case "sandbox":
		if options != nil && options.Session != nil {
			options.Session.LastGoodEndpoint = ""
		}
		return []string{sandbox}, nil
	case "production":
		if options != nil && options.Session != nil {
			options.Session.LastGoodEndpoint = ""
		}
		return []string{production}, nil
	}
	fallbacks := []string{production, sandbox}
	if options != nil && options.Session != nil {
		if last := options.Session.LastGoodEndpoint; last == production || last == sandbox {
			return []string{last, otherAntigravityEndpoint(last, production, sandbox)}, nil
		}
	}
	return fallbacks, nil
}

func otherAntigravityEndpoint(last, production, sandbox string) string {
	if last == production {
		return sandbox
	}
	return production
}

// ccaRecordEndpointSuccess remembers the host that completed a request, for the next auto-mode request.
func ccaRecordEndpointSuccess(options *GoogleCloudCodeOptions, endpoint string) {
	if options == nil || options.Session == nil || ccaEndpointMode(options) != "auto" {
		return
	}
	options.Session.LastGoodEndpoint = endpoint
}

// ccaApplyExtraBody merges extra members into the encoded request body. Members already built keep their
// positions; new ones append, matching `{ ...built, ...extra }`.
func ccaApplyExtraBody(body []byte, extra map[string]any) ([]byte, error) {
	if len(extra) == 0 {
		return body, nil
	}
	members, err := jsonSpreadMembers(body)
	if err != nil {
		return nil, err
	}
	index := make(map[string]int, len(members))
	for i, member := range members {
		index[member.key] = i
	}
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	slicesSortStrings(keys)
	for _, key := range keys {
		encoded, err := marshalRequestBody(extra[key])
		if err != nil {
			return nil, err
		}
		if at, ok := index[key]; ok {
			members[at].value = encoded
			continue
		}
		index[key] = len(members)
		members = append(members, jsonMember{key: key, value: encoded})
	}
	return jsonMembersRaw(members)
}

// jsonMembersRaw writes members in order.
func jsonMembersRaw(members []jsonMember) ([]byte, error) {
	out := []byte{'{'}
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

func slicesSortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// ccaUsageLabel is the Antigravity usage bucket label. The label MUST be a JSON string: OMP defaults it to
// String(isClaude), and an object there produces `used_claude: {}`, which the backend rejects with 403
// "You do not have a valid license of this product". A configured value that is not a string is ignored in
// favour of that default rather than serialized as an object.
func ccaUsageLabel(class ccaIdentityClass, options *GoogleCloudCodeOptions) string {
	if options != nil && options.UsageLabel != nil {
		return *options.UsageLabel
	}
	if options != nil {
		if raw, ok := options.ExtraBody["antigravityUsageLabel"]; ok {
			if text, ok := raw.(string); ok {
				return text
			}
			// A non-string label is never written: the wire form stays the provider's own string.
			delete(options.ExtraBody, "antigravityUsageLabel")
		}
	}
	return strconv.FormatBool(class == ccaIdentityAnthropic)
}

// ─── Per-account wire profiles and effort-tier families ──────────────────────

// ccaAntigravityFamily routes one logical Antigravity id to the wire SKU each thinking level selects.
type ccaAntigravityFamily struct {
	// LogicalID is the collapsed id the roster lists.
	LogicalID string
	// Tiers maps an effort to the wire id that serves it.
	Tiers map[ModelThinkingLevel]string
	// Default is the wire id a request without an effort uses.
	Default string
}

// ccaAntigravityAccount is one account's discovered roster constants: the wire profiles of every SKU it
// advertised and the effort-tier families those SKUs collapse into. Keying by account keeps one account's
// SKU caps and model_enum tokens from leaking into another's requests.
type ccaAntigravityAccount struct {
	Profiles map[string]ccaAntigravityWireProfile
	Families map[string]ccaAntigravityFamily
}

var ccaAntigravityAccounts struct {
	mu       sync.RWMutex
	accounts map[string]*ccaAntigravityAccount
}

// ccaAccountKeyOf is the serving credential's identity: the account id, else its email, else its project.
// It is the key OMP uses for per-account request constants (oauthAccountKey).
func ccaAccountKeyOf(credentials ccaCredentials) string {
	for _, value := range []string{credentials.AccountID, credentials.Email, credentials.ProjectID} {
		if value != "" {
			return value
		}
	}
	return ""
}

// ccaAccountProfiles returns the account's table, creating it on first use.
func ccaAccountProfiles(accountKey string, create bool) *ccaAntigravityAccount {
	ccaAntigravityAccounts.mu.Lock()
	defer ccaAntigravityAccounts.mu.Unlock()
	if ccaAntigravityAccounts.accounts == nil {
		if !create {
			return nil
		}
		ccaAntigravityAccounts.accounts = map[string]*ccaAntigravityAccount{}
	}
	if account, ok := ccaAntigravityAccounts.accounts[accountKey]; ok {
		return account
	}
	if !create {
		return nil
	}
	account := &ccaAntigravityAccount{Profiles: map[string]ccaAntigravityWireProfile{}, Families: map[string]ccaAntigravityFamily{}}
	ccaAntigravityAccounts.accounts[accountKey] = account
	return account
}

// ccaRecordAccountProfile records one discovered SKU's constants under its account.
func ccaRecordAccountProfile(accountKey, wireID string, profile ccaAntigravityWireProfile) {
	if accountKey == "" {
		return
	}
	account := ccaAccountProfiles(accountKey, true)
	account.Profiles[wireID] = profile
}

// ccaRecordAccountFamily records one collapsed effort-tier family under its account.
func ccaRecordAccountFamily(accountKey string, family ccaAntigravityFamily) {
	if accountKey == "" || len(family.Tiers) == 0 {
		return
	}
	account := ccaAccountProfiles(accountKey, true)
	account.Families[family.LogicalID] = family
}

// ccaAccountProfile reads one SKU's constants for an account.
func ccaAccountProfile(accountKey, wireID string) (ccaAntigravityWireProfile, bool) {
	account := ccaAccountProfiles(accountKey, false)
	if account == nil {
		return ccaAntigravityWireProfile{}, false
	}
	profile, ok := account.Profiles[wireID]
	return profile, ok
}

// ccaAccountFamily reads one account's collapsed family.
func ccaAccountFamily(accountKey, logicalID string) (ccaAntigravityFamily, bool) {
	account := ccaAccountProfiles(accountKey, false)
	if account == nil {
		return ccaAntigravityFamily{}, false
	}
	family, ok := account.Families[logicalID]
	return family, ok
}

// ccaTierSuffixes are the effort-tier suffixes Antigravity appends to a base SKU id.
var ccaTierSuffixes = []string{"-extra-low", "-minimal", "-low", "-medium", "-high", "-tiered"}

// ccaTierOfSuffix maps a suffix to the effort it serves.
var ccaTierOfSuffix = map[string]ModelThinkingLevel{
	"-extra-low": ThinkingMinimal,
	"-minimal":   ThinkingMinimal,
	"-low":       ThinkingLow,
	"-medium":    ThinkingMedium,
	"-high":      ThinkingHigh,
	"-tiered":    ThinkingMax,
}

// ccaSplitTierSuffix splits a wire id into its logical base and effort. ok is false for a non-tier id.
func ccaSplitTierSuffix(wireID string) (string, ModelThinkingLevel, bool) {
	for _, suffix := range ccaTierSuffixes {
		if base, found := strings.CutSuffix(wireID, suffix); found && base != "" {
			return base, ccaTierOfSuffix[suffix], true
		}
	}
	return wireID, "", false
}

// ccaCollapseTierFamilies groups a discovered roster's effort-tier SKUs into logical families. Members
// share a base id; the family lists the wire id each effort selects, and the emitted logical id is the base.
// The returned keep set is the ids to list; familyOf maps every member to its family.
func ccaCollapseTierFamilies(ids []string) (keep []string, families map[string]ccaAntigravityFamily) {
	families = map[string]ccaAntigravityFamily{}
	members := map[string][]struct {
		wireID string
		effort ModelThinkingLevel
	}{}
	for _, id := range ids {
		base, effort, ok := ccaSplitTierSuffix(id)
		if !ok {
			continue
		}
		members[base] = append(members[base], struct {
			wireID string
			effort ModelThinkingLevel
		}{wireID: id, effort: effort})
	}
	collapsed := map[string]struct{}{}
	for base, list := range members {
		if len(list) < 2 {
			continue
		}
		family := ccaAntigravityFamily{LogicalID: base, Tiers: map[ModelThinkingLevel]string{}}
		for _, member := range list {
			family.Tiers[member.effort] = member.wireID
			collapsed[member.wireID] = struct{}{}
		}
		// The default is the medium SKU, else the highest effort the family serves.
		if medium, ok := family.Tiers[ThinkingMedium]; ok {
			family.Default = medium
		} else {
			family.Default = family.Tiers[ThinkingHigh]
			if family.Default == "" {
				family.Default = family.Tiers[ThinkingMax]
			}
			if family.Default == "" {
				family.Default = list[0].wireID
			}
		}
		families[base] = family
	}
	seen := map[string]struct{}{}
	for _, id := range ids {
		if _, isMember := collapsed[id]; isMember {
			continue
		}
		if _, present := seen[id]; present {
			continue
		}
		seen[id] = struct{}{}
		keep = append(keep, id)
	}
	// A family emits its logical row whether or not the roster listed that id itself.
	for base := range families {
		if _, present := seen[base]; present {
			continue
		}
		seen[base] = struct{}{}
		keep = append(keep, base)
	}
	slicesSortStrings(keep)
	return keep, families
}

// ccaRouteWireID selects the wire SKU for a request: an explicit override wins, else the family entry for
// the requested effort, else the logical id itself. reviewed reports that the selection came from an
// effort-routed family, whose SKU then carries its own thinking strength.
func ccaRouteWireID(accountKey, logicalID string, options *GoogleCloudCodeOptions, level ModelThinkingLevel) (wireID string, reviewed bool) {
	wireID = logicalID
	if options != nil && options.RequestModelID != "" {
		wireID = options.RequestModelID
	}
	family, ok := ccaAccountFamily(accountKey, logicalID)
	if !ok {
		return wireID, false
	}
	if options != nil && options.RequestModelID != "" {
		// An explicit wire id of the family is a routed variant.
		for _, member := range family.Tiers {
			if member == wireID {
				return wireID, true
			}
		}
		return wireID, false
	}
	if level != "" && level != ThinkingOff {
		if member, ok := family.Tiers[level]; ok {
			return member, true
		}
	}
	if family.Default != "" {
		return family.Default, true
	}
	return wireID, true
}

// ─── Claude image clamp on Antigravity ───────────────────────────────────────

const (
	ccaMaxImageDimension       = 8000
	ccaManyImageThreshold      = 20
	ccaManyImageMaxDimension   = 2000
	ccaMaxImagePayloadBytes    = 10 * 1024 * 1024
	ccaAntigravityImagePayload = 5 * 1024 * 1024
	ccaImagePayloadHeadroom    = 0.9
)

// ccaImageCaps are the per-request image limits of one Claude reader on Antigravity.
type ccaImageCaps struct {
	maxDimension int
	payload      int
}

// ccaImageCapsFor is the source's caps: Claude behind Antigravity rejects an image over 5 MiB and an edge
// over 8000 px, and past twenty images any edge over 2000 px.
func ccaImageCapsFor(class ccaIdentityClass, isAntigravity bool, imageCount int) ccaImageCaps {
	maxDimension := ccaMaxImageDimension
	payload := ccaMaxImagePayloadBytes
	if isAntigravity && class == ccaIdentityAnthropic {
		payload = ccaAntigravityImagePayload
	}
	if imageCount > ccaManyImageThreshold {
		maxDimension = min(ccaManyImageMaxDimension, maxDimension)
	}
	return ccaImageCaps{maxDimension: maxDimension, payload: int(float64(payload) * ccaImagePayloadHeadroom)}
}

// ccaCountImages counts the image blocks of a transcript.
func ccaCountImages(transcript TranscriptContext) int {
	count := 0
	for _, message := range collapsedMessages(transcript) {
		switch message := message.(type) {
		case UserMessage:
			if blocks, ok := message.Content.(UserContentBlocks); ok {
				for _, block := range blocks {
					if _, ok := block.(ImageContent); ok {
						count++
					}
				}
			}
		case ToolResultMessage:
			for _, block := range message.Content {
				if _, ok := block.(ImageContent); ok {
					count++
				}
			}
		}
	}
	return count
}

// ccaClampImageContext is prepareAnthropicImageContext for the Cloud Code Assist reader: an image over the
// host's dimension or payload contract is decoded, shrunk and re-encoded before it reaches the wire. A
// reader without the contract (Gemini on the same gateway) keeps images as sent.
func ccaClampImageContext(model *Model, transcript TranscriptContext, class ccaIdentityClass, isAntigravity bool) TranscriptContext {
	if !slicesContainsModelInput(model.Input, ModelInputImage) {
		return transcript
	}
	count := ccaCountImages(transcript)
	if count == 0 {
		return transcript
	}
	caps := ccaImageCapsFor(class, isAntigravity, count)
	messages := collapsedMessages(transcript)
	clamped := make([]Message, len(messages))
	copy(clamped, messages)
	changed := false
	for i, message := range clamped {
		switch message := message.(type) {
		case UserMessage:
			blocks, ok := message.Content.(UserContentBlocks)
			if !ok {
				continue
			}
			next := make(UserContentBlocks, len(blocks))
			copy(next, blocks)
			local := false
			for j, block := range next {
				image, ok := block.(ImageContent)
				if !ok {
					continue
				}
				if clamped, ok := ccaClampImage(image, caps); ok {
					next[j] = clamped
					local = true
				}
			}
			if local {
				message.Content = next
				clamped[i] = message
				changed = true
			}
		case ToolResultMessage:
			next := make([]ToolResultMessageContent, len(message.Content))
			copy(next, message.Content)
			local := false
			for j, block := range next {
				image, ok := block.(ImageContent)
				if !ok {
					continue
				}
				if clamped, ok := ccaClampImage(image, caps); ok {
					next[j] = clamped
					local = true
				}
			}
			if local {
				message.Content = next
				clamped[i] = message
				changed = true
			}
		}
	}
	if !changed {
		return transcript
	}
	return NormalizeContext(Context{Messages: clamped})
}

// ccaClampImage re-encodes one base64 image when it exceeds the caps.
func ccaClampImage(block ImageContent, caps ccaImageCaps) (ImageContent, bool) {
	decoded, err := base64.StdEncoding.DecodeString(block.Data)
	if err != nil {
		if raw, rawErr := base64.RawStdEncoding.DecodeString(block.Data); rawErr == nil {
			decoded = raw
		} else {
			return block, false
		}
	}
	source, format, err := image.Decode(bytes.NewReader(decoded))
	if err != nil || format == "" {
		return block, false
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	overDimensions := width > caps.maxDimension || height > caps.maxDimension
	overPayload := len(block.Data) > caps.payload
	if !overDimensions && !overPayload {
		return block, false
	}
	current := source
	for attempt := 0; attempt < 8; attempt++ {
		bounds = current.Bounds()
		width, height := bounds.Dx(), bounds.Dy()
		if width > caps.maxDimension || height > caps.maxDimension {
			scale := min(float64(caps.maxDimension)/float64(width), float64(caps.maxDimension)/float64(height))
			width = max(1, int(float64(width)*scale))
			height = max(1, int(float64(height)*scale))
			current = ccaResizeImage(current, width, height)
		}
		if encoded, ok := ccaEncodeWithinPayload(current, block.MimeType, format, caps.payload); ok {
			return ImageContent{MimeType: encoded.mime, Data: encoded.data}, true
		}
		bounds = current.Bounds()
		if bounds.Dx() <= 1 && bounds.Dy() <= 1 {
			break
		}
		// Still over the payload budget: shrink 25% and try again, as the Claude clamp does.
		nextWidth := max(1, int(float64(bounds.Dx())*0.75))
		nextHeight := max(1, int(float64(bounds.Dy())*0.75))
		if nextWidth == bounds.Dx() && nextHeight == bounds.Dy() {
			break
		}
		current = ccaResizeImage(current, nextWidth, nextHeight)
	}
	return block, false
}

// ccaEncodedImage is one re-encoding result.
type ccaEncodedImage struct {
	mime string
	data string
}

// ccaEncodeWithinPayload re-encodes the image at descending JPEG qualities, then as lossless PNG, keeping
// the first result under the payload budget. The source MIME decides the preference: a JPEG stays JPEG.
func ccaEncodeWithinPayload(source image.Image, sourceMIME, sourceFormat string, payload int) (ccaEncodedImage, bool) {
	encoders := []struct {
		mime string
		run  func() ([]byte, error)
	}{}
	if sourceMIME == "image/jpeg" || sourceFormat == "jpeg" {
		for _, quality := range []int{90, 80, 70, 55, 40} {
			quality := quality
			encoders = append(encoders, struct {
				mime string
				run  func() ([]byte, error)
			}{mime: "image/jpeg", run: func() ([]byte, error) {
				var buffer bytes.Buffer
				if err := jpeg.Encode(&buffer, source, &jpeg.Options{Quality: quality}); err != nil {
					return nil, err
				}
				return buffer.Bytes(), nil
			}})
		}
	}
	encoders = append(encoders, struct {
		mime string
		run  func() ([]byte, error)
	}{mime: "image/png", run: func() ([]byte, error) {
		var buffer bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&buffer, source); err != nil {
			return nil, err
		}
		return buffer.Bytes(), nil
	}})
	if sourceMIME != "image/jpeg" && sourceFormat != "jpeg" {
		for _, quality := range []int{90, 80, 70, 55, 40} {
			quality := quality
			encoders = append(encoders, struct {
				mime string
				run  func() ([]byte, error)
			}{mime: "image/jpeg", run: func() ([]byte, error) {
				var buffer bytes.Buffer
				if err := jpeg.Encode(&buffer, source, &jpeg.Options{Quality: quality}); err != nil {
					return nil, err
				}
				return buffer.Bytes(), nil
			}})
		}
	}
	for _, encoder := range encoders {
		encoded, err := encoder.run()
		if err != nil {
			continue
		}
		data := base64.StdEncoding.EncodeToString(encoded)
		if len(data) > payload {
			continue
		}
		return ccaEncodedImage{mime: encoder.mime, data: data}, true
	}
	return ccaEncodedImage{}, false
}

// ccaResizeImage scales an image with nearest-neighbour sampling: the clamp only has to make an oversized
// attachment acceptable, and the standard library carries no resampler.
func ccaResizeImage(source image.Image, width, height int) image.Image {
	bounds := source.Bounds()
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	if bounds.Dx() == 0 || bounds.Dy() == 0 || width == 0 || height == 0 {
		return target
	}
	for y := range height {
		sourceY := bounds.Min.Y + y*bounds.Dy()/height
		for x := range width {
			sourceX := bounds.Min.X + x*bounds.Dx()/width
			target.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	return target
}

// ccaThinkingLevelOfRequest is the logical effort a request asks for: the explicit reasoning level, else
// the raw Google thinking level mapped down, else empty (the model's default).
func ccaThinkingLevelOfRequest(options StreamOptions) ModelThinkingLevel {
	if options.Thinking != "" {
		return ModelThinkingLevel(options.Thinking)
	}
	if raw := options.GoogleThinking; raw != nil && raw.Enabled && raw.Level != "" {
		switch raw.Level {
		case GoogleThinkingLevelMinimal:
			return ThinkingMinimal
		case GoogleThinkingLevelLow:
			return ThinkingLow
		case GoogleThinkingLevelMedium:
			return ThinkingMedium
		case GoogleThinkingLevelHigh:
			return ThinkingHigh
		}
	}
	return ""
}

// ccaReviewedTierThinking applies a reviewed tier SKU's own thinking strength over the requested effort:
// the effort only chose the SKU, and the SKU advertises a level or a budget of its own.
func ccaReviewedTierThinking(config *geminiThinkingConfig, profile ccaAntigravityWireProfile, reviewed bool) *geminiThinkingConfig {
	if !reviewed {
		return config
	}
	includeThoughts := new(true)
	if config != nil && config.IncludeThoughts != nil {
		includeThoughts = config.IncludeThoughts
	}
	if profile.ThinkingLevel != "" {
		return &geminiThinkingConfig{IncludeThoughts: includeThoughts, ThinkingLevel: profile.ThinkingLevel}
	}
	if profile.ThinkingBudget != nil {
		return &geminiThinkingConfig{IncludeThoughts: includeThoughts, ThinkingBudget: profile.ThinkingBudget}
	}
	return config
}
