package ai

// Devin (Codeium Cascade) wire codec: the Connect/protobuf pieces shared by
// the devin-agent transport, account model discovery, login identity, and the
// usage probe. Ports the hand-rolled codec of omp's
// packages/ai/src/providers/devin.ts, packages/catalog/src/wire/devin.ts and
// packages/catalog/src/wire/devin-proto.ts onto PiG's existing devinPbWriter /
// devinPbReader primitives (originally in usage_provider_xai.go).
//
// Ports packages/catalog/src/wire/devin.ts (constants, credential encoding,
// client metadata), packages/catalog/src/wire/devin-proto.ts (message shapes),
// packages/ai/src/providers/connect-frame.ts (envelope codec) and
// packages/ai/src/utils/deterministic-id.ts.

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"
)

const (
	devinProviderName   = "devin"
	devinDefaultBaseURL = "https://server.codeium.com"
	devinUserStatusPath = "/exa.seat_management_pb.SeatManagementService/GetUserStatus"

	devinAuthPath             = "/exa.auth_pb.AuthService/GetUserJwt"
	devinAssignModelPath      = "/exa.api_server_pb.ApiServerService/AssignModel"
	devinChatMessagePath      = "/exa.api_server_pb.ApiServerService/GetChatMessage"
	devinCliModelConfigsPath  = "/exa.api_server_pb.ApiServerService/GetCliModelConfigs"
	devinSessionPrefix        = "devin-session-token$"
	devinCliVersion           = "3000.12.57"
	devinMicrosPerUSD         = 1_000_000
	devinBillingQuota         = 2
	devinMinCompressedBody    = 1024
	devinMaxConnectFrameBytes = 16 * 1024 * 1024
	devinMaxErrorDetailChars  = 4096
	devinToolCallIDMaxLength  = 40
)

// devinRuntimeOS is Metadata.os: process.platform fixed for the process lifetime.
func devinRuntimeOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// devinCredentialToken reads the stored token of either credential kind.
func devinCredentialToken(credential UsageCredential) string {
	raw := credential.APIKey
	if credential.Type == CredentialOAuth {
		raw = credential.AccessToken
	}
	return trimJSWhitespace(raw)
}

// encodeDevinCredential prefixes a bare JWT-shaped session token; every other
// key, legacy sk-ws- ones included, goes out as stored.
func encodeDevinCredential(apiKey string) string {
	key := trimJSWhitespace(apiKey)
	if key == "" || strings.HasPrefix(key, devinSessionPrefix) {
		return key
	}
	if len(strings.Split(key, ".")) == 3 {
		return devinSessionPrefix + key
	}
	return key
}

// devinPbWriter writes protobuf wire bytes.
type devinPbWriter struct {
	buffer []byte
}

// appendVarint appends a base-128 varint.
func (w *devinPbWriter) appendVarint(value uint64) {
	for value >= 0x80 {
		w.buffer = append(w.buffer, byte(value)|0x80)
		value >>= 7
	}
	w.buffer = append(w.buffer, byte(value))
}

// appendTag appends a field tag.
func (w *devinPbWriter) appendTag(field int, wireType int) {
	w.appendVarint(uint64(field)<<3 | uint64(wireType))
}

// appendString appends one length-delimited string field; empty strings are omitted.
func (w *devinPbWriter) appendString(field int, value string) {
	if value == "" {
		return
	}
	w.appendTag(field, 2)
	w.appendVarint(uint64(len(value)))
	w.buffer = append(w.buffer, value...)
}

// appendMessage appends one length-delimited embedded message.
func (w *devinPbWriter) appendMessage(field int, message []byte) {
	if len(message) == 0 {
		return
	}
	w.appendTag(field, 2)
	w.appendVarint(uint64(len(message)))
	w.buffer = append(w.buffer, message...)
}

// appendBool appends a varint bool field.
func (w *devinPbWriter) appendBool(field int, value bool) {
	if !value {
		return
	}
	w.appendTag(field, 0)
	w.appendVarint(1)
}

// appendVarintField appends one varint field, including zero and negative-as-uint64 values.
func (w *devinPbWriter) appendVarintField(field int, value uint64) {
	w.appendTag(field, 0)
	w.appendVarint(value)
}

// appendDoubleField appends a fixed64 double field.
func (w *devinPbWriter) appendDoubleField(field int, value float64) {
	if value == 0 {
		return
	}
	w.appendTag(field, 1)
	w.buffer = binary.LittleEndian.AppendUint64(w.buffer, math.Float64bits(value))
}

// devinPbReader walks protobuf wire bytes.
type devinPbReader struct {
	data  []byte
	index int
}

// readVarint reads one base-128 varint.
func (r *devinPbReader) readVarint() (uint64, bool) {
	var result uint64
	var shift uint
	for {
		if r.index >= len(r.data) {
			return 0, false
		}
		value := r.data[r.index]
		r.index++
		result |= uint64(value&0x7f) << shift
		if value < 0x80 {
			return result, true
		}
		shift += 7
		if shift > 63 {
			return 0, false
		}
	}
}

// readBytes reads one length-delimited payload.
func (r *devinPbReader) readBytes() ([]byte, bool) {
	length, ok := r.readVarint()
	if !ok || uint64(len(r.data)-r.index) < length {
		return nil, false
	}
	payload := r.data[r.index : r.index+int(length)]
	r.index += int(length)
	return payload, true
}

// readTag reads one field tag, returning the field number and wire type.
func (r *devinPbReader) readTag() (field int, wireType int, ok bool) {
	tag, ok := r.readVarint()
	if !ok {
		return 0, 0, false
	}
	return int(tag >> 3), int(tag & 7), true
}

// readFixed64 reads one fixed64 little-endian value.
func (r *devinPbReader) readFixed64() (uint64, bool) {
	if len(r.data)-r.index < 8 {
		return 0, false
	}
	value := binary.LittleEndian.Uint64(r.data[r.index:])
	r.index += 8
	return value, true
}

// readFixed32 reads one fixed32 little-endian value.
func (r *devinPbReader) readFixed32() (uint32, bool) {
	if len(r.data)-r.index < 4 {
		return 0, false
	}
	value := binary.LittleEndian.Uint32(r.data[r.index:])
	r.index += 4
	return value, true
}

// skip advances past one field of an unknown wire type.
func (r *devinPbReader) skip(wireType int) bool {
	switch wireType {
	case 0:
		_, ok := r.readVarint()
		return ok
	case 1:
		_, ok := r.readFixed64()
		return ok
	case 2:
		_, ok := r.readBytes()
		return ok
	case 5:
		_, ok := r.readFixed32()
		return ok
	}
	return false
}

// devinReadString reads a string field payload; invalid UTF-8 reads as "".
func devinReadString(payload []byte) string {
	if !utf8.Valid(payload) {
		return ""
	}
	return string(payload)
}

// --- Metadata ---------------------------------------------------------------

// devinCLIMetadata encodes the released-CLI Metadata identity: the
// `devin-cli`/`chisel` tuple the backend gates router assignment and the CLI
// model surface on (wire/devin.ts DEVIN_CLI_METADATA).
func devinCLIMetadata(apiKey, userJwt string) []byte {
	w := &devinPbWriter{}
	w.appendString(1, "devin-cli") // ideName
	w.appendString(28, "chisel")   // ideType
	w.appendString(7, devinCliVersion)
	w.appendString(12, "chisel") // extensionName
	w.appendString(2, devinCliVersion)
	w.appendString(3, encodeDevinCredential(apiKey)) // apiKey
	w.appendString(4, "en")                          // locale
	w.appendString(5, devinRuntimeOS())              // os
	w.appendString(21, userJwt)                      // userJwt
	return w.buffer
}

// devinDiscoveryMetadata encodes the dev-channel identity the native CLI
// announces for GetCliModelConfigs (wire/devin.ts DEVIN_DISCOVERY_METADATA).
func devinDiscoveryMetadata(apiKey string) []byte {
	w := &devinPbWriter{}
	w.appendString(1, "chisel") // ideName
	w.appendString(12, "chisel")
	w.appendString(7, "0.0.0-dev")
	w.appendString(2, "0.0.0-dev")
	w.appendString(3, encodeDevinCredential(apiKey)) // apiKey
	w.appendString(4, "en")
	w.appendString(5, devinRuntimeOS())
	// supported_model_displays (field 30): MODEL_ROUTER(3), QUICK_REVIEW(4),
	// plus wire values 6, 7, 8 the vendored enum predates.
	for _, display := range []int{3, 4, 6, 7, 8} {
		w.appendVarintField(30, uint64(display))
	}
	return w.buffer
}

// --- Requests ----------------------------------------------------------------

// encodeDevinMetadataRequest encodes the { metadata=1 } request shape shared
// by GetUserStatusRequest and GetUserJwtRequest.
func encodeDevinMetadataRequest(apiKey string) []byte {
	request := &devinPbWriter{}
	request.appendMessage(1, devinCLIMetadata(apiKey, ""))
	return request.buffer
}

// devinImageData encodes one ImageData { base64_data=1, mime_type=2 }.
func devinImageData(base64Data, mimeType string) []byte {
	w := &devinPbWriter{}
	w.appendString(1, base64Data)
	w.appendString(2, mimeType)
	return w.buffer
}

// devinChatToolCall encodes one ChatToolCall { id=1, name=2, arguments_json=3 }.
func devinChatToolCall(id, name, argumentsJSON string) []byte {
	w := &devinPbWriter{}
	w.appendString(1, id)
	w.appendString(2, name)
	w.appendString(3, argumentsJSON)
	return w.buffer
}

// devinChatMessagePrompt holds one ChatMessagePrompt's field values, so
// AssignModel can send the last user turn too.
type devinChatMessagePrompt struct {
	messageID       string
	source          int // ChatMessageSource
	prompt          string
	toolCallID      string
	toolResultIsErr bool
	images          [][2]string // base64Data, mimeType pairs
	thinking        string
	signature       string
	toolCalls       [][3]string // id, name, argumentsJSON
}

// encode serializes the prompt per chat.proto ChatMessagePrompt.
func (p devinChatMessagePrompt) encode() []byte {
	w := &devinPbWriter{}
	w.appendString(1, p.messageID)
	if p.source != 0 {
		w.appendVarintField(2, uint64(p.source))
	}
	w.appendString(3, p.prompt)
	for _, call := range p.toolCalls {
		w.appendMessage(6, devinChatToolCall(call[0], call[1], call[2]))
	}
	w.appendString(7, p.toolCallID)
	w.appendBool(9, p.toolResultIsErr)
	for _, image := range p.images {
		w.appendMessage(10, devinImageData(image[0], image[1]))
	}
	w.appendString(11, p.thinking)
	w.appendString(12, p.signature)
	return w.buffer
}

// devinToolDefinition encodes one ChatToolDefinition { name=1, description=2,
// json_schema_string=3, strict=12 }.
func devinToolDefinition(name, description, jsonSchema string, strict bool) []byte {
	w := &devinPbWriter{}
	w.appendString(1, name)
	w.appendString(2, description)
	w.appendString(3, jsonSchema)
	w.appendBool(12, strict)
	return w.buffer
}

// devinChatToolChoice encodes ChatToolChoice { option_name=1 }.
func devinChatToolChoice(optionName string) []byte {
	w := &devinPbWriter{}
	w.appendString(1, optionName)
	return w.buffer
}

// devinPromptCacheOptions encodes PromptCacheOptions { type=1 }.
func devinPromptCacheOptions(cacheType int) []byte {
	w := &devinPbWriter{}
	w.appendVarintField(1, uint64(cacheType))
	return w.buffer
}

// devinCompletionConfiguration encodes CompletionConfiguration for one turn.
type devinCompletionConfiguration struct {
	maxTokens    uint64
	temperature  float64
	topP         float64
	stopPatterns []string
}

func (c devinCompletionConfiguration) encode() []byte {
	w := &devinPbWriter{}
	w.appendVarintField(1, 1) // numCompletions
	w.appendVarintField(2, c.maxTokens)
	w.appendVarintField(3, 400) // maxNewlines
	w.appendDoubleField(5, c.temperature)
	w.appendVarintField(7, 40) // topK
	w.appendDoubleField(8, c.topP)
	for _, pattern := range c.stopPatterns {
		w.appendString(9, pattern)
	}
	return w.buffer
}

// devinAssignModelRequest encodes AssignModelRequest { metadata=1,
// model_router_uid=2, cascade_id=3, chat_message_prompt=5 }.
func devinAssignModelRequest(apiKey, routerUID, cascadeID string, prompt devinChatMessagePrompt) []byte {
	request := &devinPbWriter{}
	request.appendMessage(1, devinCLIMetadata(apiKey, ""))
	request.appendString(2, routerUID)
	request.appendString(3, cascadeID)
	request.appendMessage(5, prompt.encode())
	return request.buffer
}

// devinModelAssignment is a decoded ModelAssignment.
type devinModelAssignment struct {
	assignmentJWT string
	modelUID      string
}

// devinGetChatMessageRequest encodes GetChatMessageRequest. Field numbers per
// api_server.proto GetChatMessageRequest.
func devinGetChatMessageRequest(apiKey, userJwt, systemPrompt, chatModelUID, cascadeID, executionID string, prompts []devinChatMessagePrompt, tools [][]byte, disableParallelToolCalls bool, assignmentJWT string, config devinCompletionConfiguration) []byte {
	request := &devinPbWriter{}
	request.appendMessage(1, devinCLIMetadata(apiKey, userJwt))
	request.appendString(2, systemPrompt) // prompt
	for _, prompt := range prompts {
		request.appendMessage(3, prompt.encode())
	}
	request.appendVarintField(7, 5) // requestType CASCADE
	request.appendMessage(8, config.encode())
	for _, tool := range tools {
		request.appendMessage(10, tool)
	}
	request.appendBool(11, disableParallelToolCalls)
	request.appendMessage(12, devinChatToolChoice("auto"))
	request.appendMessage(13, devinPromptCacheOptions(1)) // EPHEMERAL
	request.appendString(16, cascadeID)
	request.appendVarintField(20, 1) // plannerMode DEFAULT
	request.appendString(21, chatModelUID)
	request.appendString(22, executionID)
	request.appendString(26, assignmentJWT)
	return request.buffer
}

// devinGetCliModelConfigsRequest encodes GetCliModelConfigsRequest { metadata=1 }.
func devinGetCliModelConfigsRequest(apiKey string) []byte {
	request := &devinPbWriter{}
	request.appendMessage(1, devinDiscoveryMetadata(apiKey))
	return request.buffer
}

// --- Responses ---------------------------------------------------------------

// devinGetUserJwtResponse is a decoded GetUserJwtResponse.
type devinGetUserJwtResponse struct {
	userJWT          string
	customAPIBaseURL string
}

func decodeDevinGetUserJwtResponse(data []byte) *devinGetUserJwtResponse {
	reader := &devinPbReader{data: data}
	response := &devinGetUserJwtResponse{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		switch {
		case field == 1 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			response.userJWT = devinReadString(payload)
		case field == 2 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			response.customAPIBaseURL = devinReadString(payload)
		default:
			if !reader.skip(wire) {
				return nil
			}
		}
	}
	return response
}

// decodeDevinAssignModelResponse decodes AssignModelResponse { assignment=1 }.
func decodeDevinAssignModelResponse(data []byte) *devinModelAssignment {
	reader := &devinPbReader{data: data}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		if field == 1 && wire == 2 {
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			return decodeDevinModelAssignment(payload)
		}
		if !reader.skip(wire) {
			return nil
		}
	}
	return nil
}

func decodeDevinModelAssignment(data []byte) *devinModelAssignment {
	reader := &devinPbReader{data: data}
	assignment := &devinModelAssignment{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		switch {
		case field == 1 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			assignment.assignmentJWT = devinReadString(payload)
		case field == 2 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			assignment.modelUID = devinReadString(payload)
		default:
			if !reader.skip(wire) {
				return nil
			}
		}
	}
	return assignment
}

// devinChatToolCallDelta is one decoded ChatToolCall inside delta_tool_calls.
type devinChatToolCallDelta struct {
	id            string
	name          string
	argumentsJSON string
}

// devinUsageStats is a decoded ModelUsageStats.
type devinUsageStats struct {
	inputTokens      uint64
	outputTokens     uint64
	cacheWriteTokens uint64
	cacheReadTokens  uint64
}

// devinChatMessage is one decoded GetChatMessageResponse.
type devinChatMessage struct {
	messageID      string
	deltaText      string
	deltaThinking  string
	deltaSignature string
	stopReason     int
	toolCallDeltas []devinChatToolCallDelta
	usage          *devinUsageStats
	actualModelUID string
}

func decodeDevinChatMessage(data []byte) *devinChatMessage {
	reader := &devinPbReader{data: data}
	message := &devinChatMessage{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		switch {
		case field == 1 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			message.messageID = devinReadString(payload)
		case field == 3 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			message.deltaText = devinReadString(payload)
		case field == 5 && wire == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			message.stopReason = int(value)
		case field == 6 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			if call := decodeDevinChatToolCall(payload); call != nil {
				message.toolCallDeltas = append(message.toolCallDeltas, *call)
			}
		case field == 7 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			message.usage = decodeDevinUsageStats(payload)
		case field == 9 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			message.deltaThinking = devinReadString(payload)
		case field == 10 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			message.deltaSignature = devinReadString(payload)
		case field == 23 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			message.actualModelUID = devinReadString(payload)
		default:
			if !reader.skip(wire) {
				return nil
			}
		}
	}
	return message
}

func decodeDevinChatToolCall(data []byte) *devinChatToolCallDelta {
	reader := &devinPbReader{data: data}
	call := &devinChatToolCallDelta{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		switch {
		case field == 1 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			call.id = devinReadString(payload)
		case field == 2 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			call.name = devinReadString(payload)
		case field == 3 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			call.argumentsJSON = devinReadString(payload)
		default:
			if !reader.skip(wire) {
				return nil
			}
		}
	}
	return call
}

func decodeDevinUsageStats(data []byte) *devinUsageStats {
	reader := &devinPbReader{data: data}
	usage := &devinUsageStats{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		if wire == 0 {
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			switch field {
			case 2:
				usage.inputTokens = value
			case 3:
				usage.outputTokens = value
			case 4:
				usage.cacheWriteTokens = value
			case 5:
				usage.cacheReadTokens = value
			}
			continue
		}
		if !reader.skip(wire) {
			return nil
		}
	}
	return usage
}

// --- Discovery response ------------------------------------------------------

// devinModelFeatures is a decoded ModelFeatures subset.
type devinModelFeatures struct {
	supportsImages            bool
	supportsToolCalls         bool
	supportsParallelToolCalls bool
	supportsThinking          bool
}

// devinModelInfo is a decoded ModelInfo subset.
type devinModelInfo struct {
	maxTokens       int32
	maxOutputTokens int32
	harnessUIDs     []string
	displayOption   int
	isModelRouter   bool
	modelFeatures   *devinModelFeatures
}

// devinModelDimension is a decoded ModelDimension.
type devinModelDimension struct {
	label       string
	value       float64
	denominator string
	kind        int
}

// devinModelFamilyEntry is a decoded ModelFamilyMetadataEntry: the axis key
// plus the value's order and display name.
type devinModelFamilyEntry struct {
	key   string
	order int
	name  string
}

// devinModelFamilyMetadata is a decoded ModelFamilyMetadata: the
// server-declared family label, per-axis entries, and default marker.
type devinModelFamilyMetadata struct {
	label     string
	entries   []devinModelFamilyEntry
	isDefault bool
}

// devinClientModelConfig is a decoded ClientModelConfig.
type devinClientModelConfig struct {
	modelUID        string
	label           string
	disabled        bool
	supportsImages  bool
	maxTokens       int32
	isNew           bool
	isBeta          bool
	isRecommended   bool
	description     string
	modelInfo       *devinModelInfo
	modelDimensions []devinModelDimension
	modelFamily     *devinModelFamilyMetadata
	isFamilyDefault bool
}

// devinCliModelConfigsResponse is a decoded GetCliModelConfigsResponse.
type devinCliModelConfigsResponse struct {
	configs            []devinClientModelConfig
	defaultOverrideUID string
}

func decodeDevinCliModelConfigsResponse(data []byte) *devinCliModelConfigsResponse {
	reader := &devinPbReader{data: data}
	response := &devinCliModelConfigsResponse{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		if wire != 2 {
			if !reader.skip(wire) {
				return nil
			}
			continue
		}
		payload, ok := reader.readBytes()
		if !ok {
			return nil
		}
		switch field {
		case 1:
			if config := decodeDevinClientModelConfig(payload); config != nil {
				response.configs = append(response.configs, *config)
			}
		case 3:
			response.defaultOverrideUID = decodeDevinDefaultOverrideUID(payload)
		}
	}
	return response
}

func decodeDevinDefaultOverrideUID(data []byte) string {
	reader := &devinPbReader{data: data}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return ""
		}
		if field == 3 && wire == 2 {
			payload, ok := reader.readBytes()
			if !ok {
				return ""
			}
			return devinReadString(payload)
		}
		if !reader.skip(wire) {
			return ""
		}
	}
	return ""
}

func decodeDevinClientModelConfig(data []byte) *devinClientModelConfig {
	reader := &devinPbReader{data: data}
	config := &devinClientModelConfig{}
	uid := ""
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		if wire == 0 {
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			switch field {
			case 4:
				config.disabled = value != 0
			case 5:
				config.supportsImages = value != 0
			case 15:
				config.isNew = value != 0
			case 9:
				config.isBeta = value != 0
			case 11:
				config.isRecommended = value != 0
			case 18:
				config.maxTokens = int32(value)
			case 31:
				config.isFamilyDefault = value != 0
			}
			continue
		}
		if wire != 2 {
			if !reader.skip(wire) {
				return nil
			}
			continue
		}
		payload, ok := reader.readBytes()
		if !ok {
			return nil
		}
		switch field {
		case 1:
			config.label = devinReadString(payload)
		case 22:
			uid = devinReadString(payload)
		case 27:
			config.description = devinReadString(payload)
		case 23:
			config.modelInfo = decodeDevinModelInfo(payload)
		case 30:
			config.modelFamily = decodeDevinModelFamilyMetadata(payload)
		case 32:
			if dimension := decodeDevinModelDimension(payload); dimension != nil {
				config.modelDimensions = append(config.modelDimensions, *dimension)
			}
		}
	}
	config.modelUID = uid
	return config
}

// decodeDevinModelFamilyMetadata decodes ModelFamilyMetadata
// { model_family_label=1, entries=2, is_default_model_in_family=3 }.
func decodeDevinModelFamilyMetadata(data []byte) *devinModelFamilyMetadata {
	reader := &devinPbReader{data: data}
	metadata := &devinModelFamilyMetadata{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		switch {
		case field == 1 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			metadata.label = devinReadString(payload)
		case field == 2 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			if entry := decodeDevinModelFamilyEntry(payload); entry != nil {
				metadata.entries = append(metadata.entries, *entry)
			}
		case field == 3 && wire == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			metadata.isDefault = value != 0
		default:
			if !reader.skip(wire) {
				return nil
			}
		}
	}
	return metadata
}

// decodeDevinModelFamilyEntry decodes ModelFamilyMetadataEntry
// { key=1, value=2 } where value is ModelFamilyMetadataValue { order=1, name=2 }.
func decodeDevinModelFamilyEntry(data []byte) *devinModelFamilyEntry {
	reader := &devinPbReader{data: data}
	entry := &devinModelFamilyEntry{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		switch {
		case field == 1 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			entry.key = devinReadString(payload)
		case field == 2 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			decodeDevinModelFamilyValue(payload, entry)
		default:
			if !reader.skip(wire) {
				return nil
			}
		}
	}
	return entry
}

func decodeDevinModelFamilyValue(data []byte, entry *devinModelFamilyEntry) {
	reader := &devinPbReader{data: data}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return
		}
		switch {
		case field == 1 && wire == 0:
			value, ok := reader.readVarint()
			if !ok {
				return
			}
			entry.order = int(value)
		case field == 2 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return
			}
			entry.name = devinReadString(payload)
		default:
			if !reader.skip(wire) {
				return
			}
		}
	}
}

func decodeDevinModelInfo(data []byte) *devinModelInfo {
	reader := &devinPbReader{data: data}
	info := &devinModelInfo{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		if wire == 0 {
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			switch field {
			case 4:
				info.maxTokens = int32(value)
			case 13:
				info.maxOutputTokens = int32(value)
			case 22:
				info.displayOption = int(value)
			case 25:
				info.isModelRouter = value != 0
			}
			continue
		}
		if wire != 2 {
			if !reader.skip(wire) {
				return nil
			}
			continue
		}
		payload, ok := reader.readBytes()
		if !ok {
			return nil
		}
		switch field {
		case 6:
			info.modelFeatures = decodeDevinModelFeatures(payload)
		case 20:
			info.harnessUIDs = append(info.harnessUIDs, devinReadString(payload))
		}
	}
	return info
}

func decodeDevinModelFeatures(data []byte) *devinModelFeatures {
	reader := &devinPbReader{data: data}
	features := &devinModelFeatures{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		if wire == 0 {
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			switch field {
			case 11:
				features.supportsImages = value != 0
			case 12:
				features.supportsToolCalls = value != 0
			case 15:
				features.supportsThinking = value != 0
			case 21:
				features.supportsParallelToolCalls = value != 0
			}
			continue
		}
		if !reader.skip(wire) {
			return nil
		}
	}
	return features
}

func decodeDevinModelDimension(data []byte) *devinModelDimension {
	reader := &devinPbReader{data: data}
	dimension := &devinModelDimension{}
	for reader.index < len(reader.data) {
		field, wire, ok := reader.readTag()
		if !ok {
			return nil
		}
		switch {
		case field == 1 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			dimension.label = devinReadString(payload)
		case field == 2 && wire == 5:
			raw, ok := reader.readFixed32()
			if !ok {
				return nil
			}
			dimension.value = float64(math.Float32frombits(raw))
		case field == 3 && wire == 2:
			payload, ok := reader.readBytes()
			if !ok {
				return nil
			}
			dimension.denominator = devinReadString(payload)
		case field == 6 && wire == 0:
			value, ok := reader.readVarint()
			if !ok {
				return nil
			}
			dimension.kind = int(value)
		default:
			if !reader.skip(wire) {
				return nil
			}
		}
	}
	return dimension
}

// --- Unary body decode -------------------------------------------------------

// decodeDevinUnaryMessage decodes a unary protobuf body: raw first, then gzip.
func decodeDevinUnaryMessage[T any](data []byte, decode func([]byte) *T) *T {
	if decoded := decode(data); decoded != nil || len(data) == 0 {
		return decoded
	}
	decompressed, err := devinGunzip(data)
	if err != nil {
		return nil
	}
	return decode(decompressed)
}

// --- Connect envelope --------------------------------------------------------

const (
	devinConnectCompressedFlag = 0x01
	devinConnectEndStreamFlag  = 0x02
	devinConnectHeaderBytes    = 5
)

// devinFrameConnectMessage wraps one Connect message: 1 flag byte + 4-byte
// big-endian length + payload.
func devinFrameConnectMessage(payload []byte, flags int) []byte {
	frame := make([]byte, devinConnectHeaderBytes+len(payload))
	frame[0] = byte(flags)
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[devinConnectHeaderBytes:], payload)
	return frame
}

// devinConnectFrame is one decoded Connect envelope.
type devinConnectFrame struct {
	flags   int
	payload []byte
}

// devinConnectFrameDecoder incrementally decodes Connect envelopes with a
// payload cap enforced before buffering (connect-frame.ts ConnectFrameDecoder).
type devinConnectFrameDecoder struct {
	buf     []byte
	maxSize int
}

func newDevinConnectFrameDecoder() *devinConnectFrameDecoder {
	return &devinConnectFrameDecoder{maxSize: devinMaxConnectFrameBytes}
}

// decode buffers chunk and returns every frame now complete, in wire order.
func (d *devinConnectFrameDecoder) decode(chunk []byte) ([]devinConnectFrame, error) {
	if len(chunk) > 0 {
		d.buf = append(d.buf, chunk...)
	}
	var frames []devinConnectFrame
	for len(d.buf) >= devinConnectHeaderBytes {
		length := int(binary.BigEndian.Uint32(d.buf[1:5]))
		if length > d.maxSize {
			return frames, fmt.Errorf("Devin Connect frame length %d exceeds %d-byte cap", length, d.maxSize)
		}
		frameBytes := devinConnectHeaderBytes + length
		if len(d.buf) < frameBytes {
			break
		}
		frame := devinConnectFrame{flags: int(d.buf[0]), payload: d.buf[devinConnectHeaderBytes:frameBytes]}
		frames = append(frames, frame)
		d.buf = d.buf[frameBytes:]
	}
	return frames, nil
}

// devinGunzip decompresses a payload, refusing output past the frame cap.
func devinGunzip(payload []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	decompressed, err := io.ReadAll(io.LimitReader(reader, devinMaxConnectFrameBytes+1))
	if err == nil && len(decompressed) > devinMaxConnectFrameBytes {
		err = fmt.Errorf("Devin payload exceeds %d-byte cap after decompression", devinMaxConnectFrameBytes)
	}
	return decompressed, err
}

// devinDeterministicUUID formats the leading 128 bits of seed's SHA-256 as an
// 8-4-4-4-12 hex UUID shape (deterministic-id.ts).
func devinDeterministicUUID(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	hex := hex.EncodeToString(digest[:16])
	return hex[0:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" + hex[16:20] + "-" + hex[20:32]
}

// --- Errors ------------------------------------------------------------------

var devinHTMLBodyPattern = regexp.MustCompile(`(?i)^\s*(?:<!doctype\s+html\b|<html\b)`)

// devinErrorDetail extracts a bounded human error without leaking proxy HTML
// or binary protobuf (devin.ts devinErrorDetail).
func devinErrorDetail(contentType string, payload []byte) string {
	if !utf8.Valid(payload) || strings.Contains(strings.ToLower(contentType), "text/html") {
		return ""
	}
	text := strings.TrimSpace(string(payload))
	if text == "" {
		return ""
	}
	var decoded any
	if json.Unmarshal(payload, &decoded) == nil {
		if object, ok := decoded.(map[string]any); ok {
			switch errValue := object["error"].(type) {
			case map[string]any:
				if message, ok := errValue["message"].(string); ok {
					text = strings.TrimSpace(message)
				}
			case string:
				text = strings.TrimSpace(errValue)
			default:
				if message, ok := object["message"].(string); ok {
					text = strings.TrimSpace(message)
				}
			}
		}
	}
	normalized := strings.Join(strings.Fields(text), " ")
	if normalized == "" || devinHTMLBodyPattern.MatchString(normalized) {
		return ""
	}
	// Suppress text sanitizeText would alter (control chars, malformed Unicode).
	if strings.IndexFunc(normalized, func(r rune) bool {
		return r < 0x20 && r != '\t' || r == 0x7f || r >= 0x80 && r <= 0x9f
	}) >= 0 {
		return ""
	}
	if len(normalized) <= devinMaxErrorDetailChars {
		return normalized
	}
	// Truncate on a code-point boundary.
	cut := devinMaxErrorDetailChars
	for cut > 0 && !utf8.ValidString(normalized[:cut]) {
		cut--
	}
	return normalized[:cut]
}

// devinHTTPError builds "Devin <op> error <status>: <detail>".
func devinHTTPError(operation string, statusCode int, statusText, contentType string, payload []byte, headers ...http.Header) error {
	status := fmt.Sprintf("%d", statusCode)
	if statusText != "" {
		status += " " + statusText
	}
	detail := devinErrorDetail(contentType, payload)
	if detail != "" {
		return NewProviderError(fmt.Sprintf("Devin %s error %s: %s", operation, status, detail), &statusCode, nil, headers...)
	}
	return NewProviderError(fmt.Sprintf("Devin %s error %s", operation, status), &statusCode, nil, headers...)
}

// devinTrailerError is a parsed Connect end-stream error.
type devinTrailerError struct {
	code    string
	message string
	detail  string
}

// readDevinConnectTrailerError parses a Connect end-of-stream JSON trailer.
func readDevinConnectTrailerError(text string) *devinTrailerError {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(text), &parsed) != nil {
		return nil
	}
	errValue, ok := parsed["error"].(map[string]any)
	if !ok {
		return nil
	}
	code, _ := errValue["code"].(string)
	message, _ := errValue["message"].(string)
	if code == "" && message == "" {
		return nil
	}
	trailer := &devinTrailerError{code: code, message: message}
	if details, ok := errValue["details"].([]any); ok && len(details) > 0 {
		trailer.detail = devinSummarizeTrailerDetails(details)
	}
	return trailer
}

// devinSummarizeTrailerDetails summarizes Connect error details entries
// ({type, value, debug}) into bounded evidence text.
func devinSummarizeTrailerDetails(details []any) string {
	var parts []string
	for _, entry := range details {
		record, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		typeName, _ := record["type"].(string)
		var evidence string
		if debug, present := record["debug"]; present {
			if text, ok := debug.(string); ok {
				evidence = text
			} else if encoded, err := json.Marshal(debug); err == nil {
				evidence = string(encoded)
			}
		}
		if evidence == "" {
			value, _ := record["value"].(string)
			evidence = value
		}
		switch {
		case typeName != "" && evidence != "":
			parts = append(parts, typeName+": "+evidence)
		case typeName != "":
			parts = append(parts, typeName)
		case evidence != "":
			parts = append(parts, evidence)
		}
	}
	return strings.Join(parts, "; ")
}

// Error formats the trailer error the way the provider throws it.
func (e *devinTrailerError) Error() string {
	formatted := "Devin stream error"
	if e.code != "" {
		formatted += " " + e.code
	}
	formatted += ": " + e.message
	if e.detail != "" {
		formatted += " [details: " + e.detail + "]"
	}
	return formatted
}
