package modelmetadata

// Ports packages/coding-agent/src/core/model-metadata-match.ts: safe fuzzy matching of a configured
// model id to a public catalog row.
//
// Custom endpoints publish deployment-wrapped ids (deepseek-ai/deepseek-v4.1-flash-1017,
// @cf/deepseek-ai/..., [Kiro] claude-opus-4-8, global.anthropic.claude-opus-4-8-v1:0). Stripping the
// vendor path, deployment date and cloud prefix finds the family row; the matcher then requires the
// same family, version, variant word, size and marker set, so deepseek-v4-pro never borrows
// deepseek-v4.1-flash metadata. Display names are parsed too, because first-party rows sometimes keep
// a legacy id (deepseek-flash) while their published name carries the version
// (DeepSeek V4.1 Flash 1017).

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/MichaelKinsy/PiG/ai"
)

// modelFamilies are the families the matcher recognizes inside an id or name token.
var modelFamilies = map[string]bool{
	"deepseek": true, "claude": true, "gpt": true, "gemini": true, "grok": true,
	"glm": true, "qwen": true, "kimi": true, "llama": true, "mistral": true,
	"phi": true, "command": true, "jamba": true, "minimax": true, "mimo": true,
	"doubao": true, "ernie": true, "gemma": true, "nemotron": true, "sonar": true,
	"o1": true, "o3": true, "o4": true,
}

// variantMarkers are words that change which model an id names; both sides must carry the same set.
var variantMarkers = map[string]bool{
	"thinking": true, "reasoner": true, "reasoning": true, "vision": true, "exp": true,
	"experimental": true, "preview": true, "beta": true, "alpha": true, "coder": true,
	"code": true, "chat": true, "instruct": true, "base": true, "turbo": true, "fast": true,
	"ultraspeed": true, "highspeed": true, "search": true, "distill": true, "guard": true,
	"embedding": true,
}

var (
	// sizePattern matches size parameters that distinguish models in one family (120b, 8x7b, 35b-a3b).
	sizePattern = regexp.MustCompile(`^(?:\d+(?:\.\d+)?[bmk](?:-a\d+(?:\.\d+)?b)?|\d+x\d+(?:\.\d+)?b)$`)
	// datePattern matches deployment/release date suffixes (-1017, -20250514).
	datePattern = regexp.MustCompile(`^\d{4}$|^\d{8}$`)
	// versionPattern matches a version token (v4.1, 4, 4.1.2).
	versionPattern = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)$`)
	// activeSizePattern matches the 35b / a3b halves of a MoE size such as 35b-a3b.
	activeSizePattern = regexp.MustCompile(`^(?:\d+(?:\.\d+)?[bmk]|a\d+(?:\.\d+)?b)$`)
	// wireSuffixPattern is the Bedrock-style wire suffix that names no model fact.
	wireSuffixPattern = regexp.MustCompile(`-v\d+(?::\d+)?$|:\d+$`)
	// bracketAffixPattern strips bracket tags such as [Kiro] and the CJK full-width variants.
	bracketAffixPattern = regexp.MustCompile(`^(?:\s*[\[【][^\]】]+[\]】]\s*)+|(?:\s*[\[【][^\]】]+[\]】]\s*)+$`)
	// gluedVersionPattern separates a family from a version glued onto it (qwen3.6).
	gluedVersionPattern = regexp.MustCompile(`^([a-z]+)(\d+(?:\.\d+)*)$`)
	// nonTokenPattern splits an id into tokens.
	nonTokenPattern = regexp.MustCompile(`[^a-z0-9.]+`)
)

// maxVersionParts bounds how many version components identify a model (MAX_VERSION_PARTS).
const maxVersionParts = 3

// VariantKey is the set of identity facts a catalog match must reproduce exactly.
type VariantKey struct {
	Family  string
	Version string
	Variant string
	Size    string
	Markers []string
	// Date is the deployment date; it is kept for the date check, not for equality of the model.
	Date string
	// HasVersion/HasVariant/HasSize/HasDate record whether the fact was present at all, since the
	// empty string is also a legal id token.
	HasVersion bool
	HasVariant bool
	HasSize    bool
	HasDate    bool
}

// tokens splits an id or name into lower-case tokens: the bracket affixes and the vendor path are
// dropped, the Bedrock wire suffix is removed, and every run of non-token characters separates.
func tokens(value string) []string {
	lowered := strings.ToLower(value)
	cleaned := bracketAffixPattern.ReplaceAllString(lowered, " ")
	if slash := strings.LastIndex(cleaned, "/"); slash >= 0 {
		cleaned = cleaned[slash+1:]
	}
	cleaned = wireSuffixPattern.ReplaceAllString(cleaned, "")
	parts := nonTokenPattern.Split(cleaned, -1)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// familyOf splits a token into family and any version glued to it (qwen3.6 becomes qwen + 3.6).
func familyOf(token string) (family, version string, ok bool) {
	if modelFamilies[token] {
		return token, "", true
	}
	match := gluedVersionPattern.FindStringSubmatch(token)
	if match == nil || !modelFamilies[match[1]] {
		return "", "", false
	}
	return match[1], match[2], true
}

// familiesIn splits a raw token into family/version even when the version is glued on.
func familiesIn(token string) (family, version string, ok bool) {
	if family, version, ok := familyOf(token); ok {
		return family, version, true
	}
	head, _, _ := strings.Cut(token, ".")
	return familyOf(head)
}

// keyCache holds parsed keys: ids are a bounded set and parsing runs per enrichment.
var keyCache sync.Map

// ParseVariantKey parses one id or name into identity facts; ok is false without a recognizable
// family.
func ParseVariantKey(value string) (*VariantKey, bool) {
	if cached, found := keyCache.Load(value); found {
		if cached == nil {
			return nil, false
		}
		return cached.(*VariantKey), true
	}
	parsed, ok := parseVariantKeyUncached(value)
	if !ok {
		keyCache.Store(value, nil)
		return nil, false
	}
	keyCache.Store(value, parsed)
	return parsed, true
}

func parseVariantKeyUncached(value string) (*VariantKey, bool) {
	list := tokens(value)
	family := ""
	versionParts := []string{}
	markers := map[string]bool{}
	variant := ""
	hasVariant := false
	size := ""
	hasSize := false
	date := ""
	start := -1
	// Find the family token. The whole token wins (qwen3.6 is family+version); only a dotted vendor
	// path (global.anthropic.claude-...) is split into segments.
	for index := 0; index < len(list) && family == ""; index++ {
		foundFamily, foundVersion, ok := familiesIn(list[index])
		if !ok {
			for _, segment := range strings.Split(list[index], ".") {
				if candidateFamily, candidateVersion, candidateOK := familiesIn(segment); candidateOK {
					foundFamily, foundVersion, ok = candidateFamily, candidateVersion, true
					break
				}
			}
		}
		if !ok {
			continue
		}
		family = foundFamily
		start = index
		if foundVersion != "" {
			versionParts = strings.Split(foundVersion, ".")
		}
	}
	if family == "" || start == -1 {
		return nil, false
	}
	// Version/dates/variants after the family. A bare number counts as a version even after the
	// variant word (claude-opus-4-8, deepseek-v4-flash).
	for index := start + 1; index < len(list); index++ {
		for _, part := range strings.Split(list[index], ".") {
			if part == "" {
				continue
			}
			if datePattern.MatchString(part) {
				if date == "" {
					date = part
				}
				continue
			}
			if sizePattern.MatchString(part) {
				// MoE ids join a parameter count and an active count with a hyphen (35b-a3b); the
				// second half is part of the size, not a variant word.
				if !hasSize {
					size, hasSize = part, true
				}
				if activeSizePattern.MatchString(part) && index+1 < len(list) {
					next := list[index+1]
					if activeSizePattern.MatchString(next) {
						size = part + "-" + next
						index++
					}
				}
				continue
			}
			if version := versionPattern.FindStringSubmatch(part); version != nil {
				if len(versionParts) < maxVersionParts {
					for _, piece := range strings.Split(version[1], ".") {
						number, err := strconv.Atoi(piece)
						if err != nil {
							number = 0
						}
						versionParts = append(versionParts, strconv.Itoa(number))
					}
				}
				continue
			}
			if variantMarkers[part] {
				markers[part] = true
				continue
			}
			if !hasVariant {
				variant, hasVariant = part, true
			}
		}
	}
	key := &VariantKey{
		Family:     family,
		Variant:    variant,
		Size:       size,
		Date:       date,
		HasVariant: hasVariant,
		HasSize:    hasSize,
		HasDate:    date != "",
	}
	if len(versionParts) > 0 {
		parts := versionParts
		if len(parts) > maxVersionParts {
			parts = parts[:maxVersionParts]
		}
		key.Version = strings.Join(parts, ".")
		key.HasVersion = true
	}
	for marker := range markers {
		key.Markers = append(key.Markers, marker)
	}
	sort.Strings(key.Markers)
	return key, true
}

// MergeVariantKeys merges facts from an id and a display name: the id decides variant/size, the name
// fills gaps.
func MergeVariantKeys(idKey, nameKey *VariantKey) *VariantKey {
	if idKey == nil {
		return nameKey
	}
	if nameKey == nil || nameKey.Family != idKey.Family {
		return idKey
	}
	merged := *idKey
	if !idKey.HasVersion && nameKey.HasVersion {
		merged.Version, merged.HasVersion = nameKey.Version, true
	}
	if !idKey.HasVariant && nameKey.HasVariant {
		merged.Variant, merged.HasVariant = nameKey.Variant, true
	}
	if !idKey.HasSize && nameKey.HasSize {
		merged.Size, merged.HasSize = nameKey.Size, true
	}
	if !idKey.HasDate && nameKey.HasDate {
		merged.Date, merged.HasDate = nameKey.Date, true
	}
	markerSet := map[string]bool{}
	for _, marker := range idKey.Markers {
		markerSet[marker] = true
	}
	for _, marker := range nameKey.Markers {
		markerSet[marker] = true
	}
	markers := make([]string, 0, len(markerSet))
	for marker := range markerSet {
		markers = append(markers, marker)
	}
	sort.Strings(markers)
	merged.Markers = markers
	return &merged
}

// VariantKeyFor is the identity of an id plus its published name; the one place both are merged.
func VariantKeyFor(id, name string) (*VariantKey, bool) {
	idKey, ok := ParseVariantKey(id)
	if !ok {
		if name == "" {
			return nil, false
		}
		return ParseVariantKey(name)
	}
	if name == "" {
		return idKey, true
	}
	nameKey, _ := ParseVariantKey(name)
	return MergeVariantKeys(idKey, nameKey), true
}

// VariantKeysMatch reports whether two keys describe the same model. A missing date on either side is
// wildcarded.
func VariantKeysMatch(left, right *VariantKey) bool {
	if left == nil || right == nil {
		return false
	}
	if left.Family != right.Family {
		return false
	}
	if left.HasVersion != right.HasVersion || left.Version != right.Version {
		return false
	}
	if left.HasVariant != right.HasVariant || left.Variant != right.Variant {
		return false
	}
	if left.HasSize != right.HasSize || left.Size != right.Size {
		return false
	}
	if len(left.Markers) != len(right.Markers) {
		return false
	}
	for i := range left.Markers {
		if left.Markers[i] != right.Markers[i] {
			return false
		}
	}
	// Two different release dates are different builds of the same family.
	if left.HasDate && right.HasDate && left.Date != right.Date {
		return false
	}
	return true
}

func rowKey(row ai.ModelCatalogMetadataRow) (*VariantKey, bool) {
	return VariantKeyFor(row.ID, row.Name)
}

// familyProviders lists the providers that publish a family's canonical list. Prices and limits are
// only inherited from these (or from an exact-id row of the same provider); a reseller row may quote
// a different rate, so its cost is not evidence of what this endpoint charges.
var familyProviders = map[string][]string{
	"claude":   {"anthropic"},
	"command":  {"cohere"},
	"deepseek": {"deepseek"},
	"doubao":   {"volcengine", "byteplus", "bytedance"},
	"ernie":    {"baidu", "qianfan"},
	"gemini":   {"google", "google-vertex"},
	"glm":      {"zai", "zhipu"},
	"gpt":      {"openai"},
	"gemma":    {"google"},
	"grok":     {"xai"},
	"jamba":    {"ai21"},
	"kimi":     {"moonshotai", "moonshot"},
	"llama":    {"meta", "groq", "together"},
	"mimo":     {"xiaomi"},
	"minimax":  {"minimax"},
	"mistral":  {"mistral"},
	"nemotron": {"nvidia"},
	"o1":       {"openai"},
	"o3":       {"openai"},
	"o4":       {"openai"},
	"phi":      {"microsoft"},
	"qwen":     {"qwen", "alibaba"},
	"sonar":    {"perplexity"},
}

// providerSuffixPattern strips the regional/plan suffix of zai-cn, minimax-cn and friends.
var providerSuffixPattern = regexp.MustCompile(`-cn$|-(?:token-plan|token-plan-cn|individual|individual-cn|coding-plan|plan)$`)

// normalizeProviderID maps regional and plan variants onto their owner (zai-cn is zai).
func normalizeProviderID(provider string) string {
	return providerSuffixPattern.ReplaceAllString(strings.ToLower(provider), "")
}

// IsCanonicalProvider reports whether the row's provider publishes this family's canonical metadata.
func IsCanonicalProvider(provider, family string) bool {
	normalized := normalizeProviderID(provider)
	if normalized == family || strings.HasPrefix(normalized, family+"-") {
		return true
	}
	for _, candidate := range familyProviders[family] {
		if normalized == candidate {
			return true
		}
	}
	return false
}

// catalogRowScore scores richer rows higher: more of the fields a selector shows.
func catalogRowScore(row ai.ModelCatalogMetadataRow) int {
	score := 0
	if row.Int != nil {
		score += 8
	}
	if row.Cost != nil {
		for _, rate := range []*float64{row.Cost.Input, row.Cost.Output, row.Cost.CacheRead, row.Cost.CacheWrite} {
			if rate != nil {
				score++
			}
		}
	}
	if row.ContextWindow != nil {
		score += 2
	}
	if row.MaxTokens != nil {
		score++
	}
	score += len(row.Efforts)
	return score
}

// candidateRows lists the unranked candidates for one identity: exact-id rows plus rows with the
// same identity key.
func candidateRows(target *VariantKey, modelID string, rows []ai.ModelCatalogMetadataRow) []ai.ModelCatalogMetadataRow {
	targetID := strings.ToLower(modelID)
	matches := []ai.ModelCatalogMetadataRow{}
	for _, row := range rows {
		key, ok := rowKey(row)
		if !ok {
			continue
		}
		// The catalog row carrying the identical id is authoritative even when its published name
		// adds facts its id omits (deepseek-flash = "DeepSeek V4.1 Flash 1017"): id equality is
		// stronger evidence than a name-derived version.
		if strings.ToLower(row.ID) != targetID && !VariantKeysMatch(target, key) {
			continue
		}
		matches = append(matches, row)
	}
	return matches
}

// catalogStatusRank sorts deprecated and other published statuses last; unmarked rows sort first.
func catalogStatusRank(row ai.ModelCatalogMetadataRow) int {
	switch row.Status {
	case "":
		return 0
	case "beta", "preview":
		return 1
	default:
		return 2
	}
}

// RankCatalogMetadataCandidates ranks candidates of one identity: canonical provider, exact id,
// non-deprecated, richer, then a deterministic provider/id order.
func RankCatalogMetadataCandidates(matches []ai.ModelCatalogMetadataRow, family, modelID string) {
	targetID := strings.ToLower(modelID)
	sort.SliceStable(matches, func(i, j int) bool {
		left, right := matches[i], matches[j]
		// The family's canonical row wins even when a reseller republished the exact id: its prices
		// are the model's list prices, while a reseller row quotes that gateway's own markup, which
		// says nothing about the configured endpoint.
		leftCanonical := IsCanonicalProvider(left.Provider, family)
		rightCanonical := IsCanonicalProvider(right.Provider, family)
		if leftCanonical != rightCanonical {
			return leftCanonical
		}
		leftExact := strings.ToLower(left.ID) == targetID
		rightExact := strings.ToLower(right.ID) == targetID
		if leftExact != rightExact {
			return leftExact
		}
		if leftStatus, rightStatus := catalogStatusRank(left), catalogStatusRank(right); leftStatus != rightStatus {
			return leftStatus < rightStatus
		}
		if leftScore, rightScore := catalogRowScore(left), catalogRowScore(right); leftScore != rightScore {
			return leftScore > rightScore
		}
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		return left.ID < right.ID
	})
}

// Index holds precomputed candidate lists, so enrichment never scans the whole catalog per model.
type Index struct {
	byKey     map[string][]ai.ModelCatalogMetadataRow
	byExactID map[string][]ai.ModelCatalogMetadataRow
}

// variantKeyID is the identity key shared by a target and the rows it may match.
func variantKeyID(key *VariantKey) string {
	version := ""
	if key.HasVersion {
		version = key.Version
	}
	variant := ""
	if key.HasVariant {
		variant = key.Variant
	}
	size := ""
	if key.HasSize {
		size = key.Size
	}
	return strings.Join([]string{key.Family, version, variant, size, strings.Join(key.Markers, ",")}, "\x00")
}

// buildIndex builds the lookup index for one snapshot.
func buildIndex(rows []ai.ModelCatalogMetadataRow) Index {
	index := Index{byKey: map[string][]ai.ModelCatalogMetadataRow{}, byExactID: map[string][]ai.ModelCatalogMetadataRow{}}
	for _, row := range rows {
		key, ok := rowKey(row)
		if !ok {
			continue
		}
		id := variantKeyID(key)
		index.byKey[id] = append(index.byKey[id], row)
		exact := strings.ToLower(row.ID)
		index.byExactID[exact] = append(index.byExactID[exact], row)
	}
	return index
}

// indexedCandidates lists the candidates for one model: the rows sharing its identity key plus the
// rows whose id equals the model id, deduplicated. Best-first ordering is applied by the ranker.
func indexedCandidates(index Index, modelID, modelName string) []ai.ModelCatalogMetadataRow {
	target, ok := VariantKeyFor(modelID, modelName)
	if !ok {
		return nil
	}
	// A row can sit in both buckets, so the union is deduplicated.
	seen := map[string]bool{}
	var bucket []ai.ModelCatalogMetadataRow
	add := func(row ai.ModelCatalogMetadataRow) {
		identity := row.Provider + "\x00" + row.ID
		if seen[identity] {
			return
		}
		seen[identity] = true
		bucket = append(bucket, row)
	}
	for _, row := range index.byKey[variantKeyID(target)] {
		add(row)
	}
	for _, row := range index.byExactID[strings.ToLower(modelID)] {
		add(row)
	}
	return candidateRows(target, modelID, bucket)
}

// MatchCatalogMetadataRows lists every catalog row reproducing modelID's identity, best-first.
//
// Rows from the family's own provider rank above resellers, because they carry the canonical price
// list; a reseller row is a fallback for metadata the source omits. Candidate selection never merges
// rows of different versions, sizes, variants or marker sets — those are filtered out before
// ranking.
func MatchCatalogMetadataRows(modelID, modelName string, rows []ai.ModelCatalogMetadataRow) []ai.ModelCatalogMetadataRow {
	target, ok := VariantKeyFor(modelID, modelName)
	if !ok {
		return nil
	}
	matches := candidateRows(target, modelID, rows)
	RankCatalogMetadataCandidates(matches, target.Family, modelID)
	return matches
}

// FindCatalogMetadataRow returns the catalog row that best describes modelID.
func FindCatalogMetadataRow(modelID, modelName string, rows []ai.ModelCatalogMetadataRow) (ai.ModelCatalogMetadataRow, bool) {
	matches := MatchCatalogMetadataRows(modelID, modelName, rows)
	if len(matches) == 0 {
		return ai.ModelCatalogMetadataRow{}, false
	}
	return matches[0], true
}

// ConsensusMetrics are the metrics a candidate set agrees on.
type ConsensusMetrics struct {
	Int *float64
	TPS *float64
}

// ConsensusCatalogMetrics returns the metrics every candidate that publishes them agrees on.
//
// int/tps score the model, not one host's price list, so a first-party row without a score may
// borrow one — but only when every candidate that publishes the metric agrees. Disagreement means the
// identity is ambiguous and the metric stays absent rather than guessed.
func ConsensusCatalogMetrics(candidates []ai.ModelCatalogMetadataRow) ConsensusMetrics {
	var intelligence, tps *float64
	intConflict, tpsConflict := false, false
	for _, row := range candidates {
		if row.Int != nil {
			if intelligence == nil {
				intelligence = row.Int
			} else if *intelligence != *row.Int {
				intConflict = true
			}
		}
		if row.TPS != nil {
			if tps == nil {
				tps = row.TPS
			} else if *tps != *row.TPS {
				tpsConflict = true
			}
		}
	}
	metrics := ConsensusMetrics{}
	if intelligence != nil && !intConflict {
		metrics.Int = intelligence
	}
	if tps != nil && !tpsConflict {
		metrics.TPS = tps
	}
	return metrics
}

// ResolveCatalogMetrics resolves the metrics for one model identity against the full row list.
func ResolveCatalogMetrics(modelID, modelName string, rows []ai.ModelCatalogMetadataRow) ConsensusMetrics {
	return ConsensusCatalogMetrics(MatchCatalogMetadataRows(modelID, modelName, rows))
}
