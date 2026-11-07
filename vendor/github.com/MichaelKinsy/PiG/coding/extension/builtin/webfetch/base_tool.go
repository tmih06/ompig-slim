//go:build !pig_strip_webfetch

// Package webfetch registers the `web_fetch` built-in tool: one URL rendered to
// readable text, Markdown or an image by a deterministic, credential-free
// pipeline.
//
// It is a built-in tool, not an extension: the definition registers itself with
// the extended-built-in registry in init, so `web_fetch` exists with
// --no-extensions and is active by default (settings_default_tools.go names it).
//
// The implementation lives in internal/webtools, which may not import
// internal/codingagent (that package imports this one); this file only adapts
// the definition and the tool-boundary truncation.
package webfetch

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
	"github.com/MichaelKinsy/PiG/internal/imageprocessing"
	"github.com/MichaelKinsy/PiG/internal/outputfiles"
	"github.com/MichaelKinsy/PiG/internal/webtools"
)

// ToolName is the tool's registered name.
const ToolName = "web_fetch"

// Tool-boundary output limits: the model sees the head of the document and the
// remainder is written to an artifact the read tool can page through.
const (
	maxOutputLines = 300
	maxOutputBytes = 50 * 1024
)

// inputSchema declares the tool's parameters in TypeBox's key order.
var inputSchema = json.RawMessage(`{"type":"object","required":["url"],"properties":{
	"url":{"type":"string","description":"Absolute http(s) URL, or a bare host such as www.example.com/page. Append :raw to skip text shaping or :50-80 to select lines."},
	"offset":{"type":"number","description":"First line of the rendered text to return (1-indexed)"},
	"limit":{"type":"number","description":"Maximum number of lines to return"},
	"timeoutSeconds":{"type":"number","description":"Whole-call timeout in seconds (default 30, clamped to 1-300)"}
}}`)

const description = "Fetch a URL and return its content as readable text or an image. Converts HTML to Markdown locally (no API key, no hosted service), pretty-prints JSON, renders feeds, notebooks, PDF, DOCX/PPTX/XLSX, EPUB, archive and SQLite listings. Use :raw on the URL for the unmodified body and :50-80 for a line range. Output is truncated to 300 lines or 50KB; the remainder is saved and can be read with the read tool."

const promptSnippet = "Fetch a URL as readable text, Markdown, or an image"

var promptGuidelines = []string{
	"Use web_fetch to read a web page or document instead of curl or wget.",
	"Append :raw to a URL to get the response body unchanged, and :50-80 to read a line range.",
}

// Options configures the definition.
type Options struct {
	// GetSettings returns the effective settings, read on every use (`webFetch`).
	// Nil means the defaults.
	GetSettings func() extension.Settings
}

// settings values resolved per call.
type fetchSettings struct {
	reader   string
	maxBytes int64
	timeout  time.Duration
	enabled  bool
}

// readSettings resolves the `webFetch` settings namespace.
func readSettings(settings extension.Settings) fetchSettings {
	resolved := fetchSettings{reader: "auto", maxBytes: webtools.DefaultMaxBytes, timeout: webtools.DefaultFetchTimeout, enabled: true}
	if settings == nil {
		return resolved
	}
	section, ok := settings["webFetch"].(map[string]any)
	if !ok {
		return resolved
	}
	if value, ok := section["reader"].(string); ok && strings.TrimSpace(value) != "" {
		resolved.reader = strings.TrimSpace(value)
	}
	if value, ok := section["maxBytes"].(float64); ok && value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value) {
		// Clamp before the conversion: a float beyond the int range converts to an
		// unspecified value. 2 GiB is already past the pipeline's own 1 GiB cap.
		resolved.maxBytes = int64(min(value, math.MaxInt32))
	}
	if value, ok := section["timeoutSeconds"].(float64); ok && value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value) {
		// Clamp in seconds first: multiplying a huge value by a second overflows.
		seconds := min(value, float64(webtools.MaxFetchTimeout/time.Second))
		resolved.timeout = time.Duration(seconds * float64(time.Second))
	}
	if value, ok := section["enabled"].(bool); ok {
		resolved.enabled = value
	}
	return resolved
}

// params is the tool's decoded input.
type params struct {
	URL            string   `json:"url"`
	Offset         *float64 `json:"offset,omitempty"`
	Limit          *float64 `json:"limit,omitempty"`
	TimeoutSeconds *float64 `json:"timeoutSeconds,omitempty"`
}

// Details is the metadata the tool result carries.
type Details struct {
	URL        string   `json:"url"`
	Method     string   `json:"method"`
	MediaType  string   `json:"mediaType,omitempty"`
	Title      string   `json:"title,omitempty"`
	Status     int      `json:"status,omitempty"`
	Notes      []string `json:"notes,omitempty"`
	TotalLines int      `json:"totalLines,omitempty"`
	StartLine  int      `json:"startLine,omitempty"`
	EndLine    int      `json:"endLine,omitempty"`
	Truncated  bool     `json:"truncated,omitempty"`
	// FullOutputPath holds the whole rendering when the model-visible text is
	// truncated: `read` pages through it.
	FullOutputPath string `json:"fullOutputPath,omitempty"`
}

// Definition builds the `web_fetch` tool definition.
func Definition(options Options) extension.ToolDefinition {
	return extension.ToolDefinition{
		Name:             ToolName,
		Label:            ToolName,
		Description:      description,
		PromptSnippet:    promptSnippet,
		PromptGuidelines: promptGuidelines,
		Parameters:       inputSchema,
		Annotations:      &extension.ToolAnnotations{ReadOnlyHint: new(true), OpenWorldHint: new(true)},
		Execute: func(ctx context.Context, toolCallID string, rawParams json.RawMessage, onUpdate extension.AgentToolUpdateCallback) (extension.AgentToolResult, error) {
			return Execute(ctx, rawParams, options)
		},
	}
}

// Execute fetches one URL.
func Execute(ctx context.Context, rawParams json.RawMessage, options Options) (extension.AgentToolResult, error) {
	var input params
	if err := json.Unmarshal(rawParams, &input); err != nil {
		return agent.AgentToolResult{}, fmt.Errorf("%s: invalid params: %w", ToolName, err)
	}
	if strings.TrimSpace(input.URL) == "" {
		return errorResult(fmt.Sprintf("Error: %s requires a url", ToolName), &Details{}), nil
	}
	resolved := fetchSettings{reader: "auto", maxBytes: webtools.DefaultMaxBytes, timeout: webtools.DefaultFetchTimeout, enabled: true}
	if options.GetSettings != nil {
		resolved = readSettings(options.GetSettings())
	}
	if !resolved.enabled {
		return agent.AgentToolResult{}, fmt.Errorf("web_fetch is disabled by the webFetch.enabled setting")
	}
	// An unknown reader is a configuration problem: the session should hear about it
	// once, not on every URL.
	switch resolved.reader {
	case "auto", "native":
	default:
		return agent.AgentToolResult{}, fmt.Errorf("webFetch.reader %q is not supported (use \"auto\" or \"native\")", resolved.reader)
	}

	fetchOptions := webtools.FetchOptions{Reader: resolved.reader, MaxBytes: resolved.maxBytes, Timeout: resolved.timeout}
	if input.Offset != nil && *input.Offset > 0 {
		fetchOptions.Offset = floatToInt(*input.Offset)
	}
	if input.Limit != nil && *input.Limit > 0 {
		fetchOptions.Limit = floatToInt(*input.Limit)
	}
	if input.TimeoutSeconds != nil && *input.TimeoutSeconds > 0 {
		seconds := math.Min(*input.TimeoutSeconds, float64(webtools.MaxFetchTimeout/time.Second))
		fetchOptions.Timeout = time.Duration(seconds * float64(time.Second))
	}

	// The client is request-scoped: these tools keep no session resources.
	client := webtools.NewHTTPClient()
	defer client.CloseIdleConnections()
	fetchOptions.Client = client

	result, err := webtools.Fetch(ctx, input.URL, fetchOptions)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return agent.AgentToolResult{}, ctxErr
		}
		return errorResult("Error: "+err.Error(), &Details{URL: input.URL}), nil
	}
	return render(ctx, result)
}

// render turns a fetch result into the tool result: an image block when the URL
// is an image the model accepts, otherwise truncated text plus an artifact.
func render(ctx context.Context, result webtools.FetchResult) (extension.AgentToolResult, error) {
	details := &Details{
		URL:        result.URL,
		Method:     result.Method,
		MediaType:  result.MediaType,
		Title:      result.Title,
		Status:     result.Status,
		Notes:      append([]string(nil), result.Notes...),
		TotalLines: result.TotalLines,
		StartLine:  result.StartLine,
		EndLine:    result.EndLine,
		// The response body hitting its cap is a truncation the model must know
		// about, whether or not the rendering also exceeds the output limits.
		Truncated: result.Truncated,
	}

	if result.Image != nil {
		return renderImage(ctx, result, details), nil
	}

	header := headerLine(result)
	body := result.Text
	if strings.TrimSpace(body) == "" {
		switch {
		case result.TotalLines == 0:
			body = "(the document rendered to no text)"
		default:
			body = fmt.Sprintf("(the requested line window is empty; the document has %d lines)", result.TotalLines)
		}
	}
	truncation := tools.TruncateHead(body, tools.TruncationOptions{
		MaxLines: new(maxOutputLines),
		MaxBytes: new(maxOutputBytes),
	})
	visible := truncation.Content
	var tail []string
	if warning := tools.FormatTruncationWarning(truncation); warning != "" {
		details.Truncated = true
		if path, err := outputfiles.WriteFile("pi-fetch", ".md", []byte(body)); err == nil {
			details.FullOutputPath = path
			tail = append(tail, fmt.Sprintf("%s Full output: %s (read it with offset/limit).", warning, path))
		} else {
			tail = append(tail, warning)
		}
	} else if truncation.FirstLineExceedsLimit {
		details.Truncated = true
		tail = append(tail, tools.FormatTruncationWarning(truncation))
	}
	// Report the window actually shown: the URL's line range and offset set where it
	// starts, and the output truncation may cut it before the document ends. Only
	// offer a next range that exists.
	if shown := shownWindow(result, truncation); shown.start <= shown.end && (shown.start > 1 || shown.end < result.TotalLines) {
		note := fmt.Sprintf("[Showing lines %d-%d of %d.", shown.start, shown.end, result.TotalLines)
		if shown.end < result.TotalLines {
			note += fmt.Sprintf(" Append :%d-%d to the URL for the next range.", shown.end+1, min(shown.end+maxOutputLines, result.TotalLines))
		}
		tail = append(tail, note+"]")
	}
	for _, note := range result.Notes {
		tail = append(tail, "[Note: "+note+"]")
	}

	text := header + "\n\n" + visible
	if len(tail) > 0 {
		text += "\n\n" + strings.Join(tail, "\n")
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: text}},
		Details: details,
	}, nil
}

// renderImage returns the image as an attachment when the model accepts images,
// and describes it otherwise.
func renderImage(ctx context.Context, result webtools.FetchResult, details *Details) extension.AgentToolResult {
	env, _ := agent.ToolEnvironmentFrom(ctx)
	supportsImages := env.SupportsImages == nil || *env.SupportsImages
	data, mime, hints, err := imageprocessing.ProcessImage(result.Image.Data, result.Image.MIME, supportsImages, nil)
	text := headerLine(result)
	if err != nil {
		text += "\n\n" + err.Error()
		return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: text}}, Details: details}
	}
	text += "\n\n[Image " + mime + ", " + tools.FormatSize(len(data)) + "]"
	if hints != "" {
		text += "\n" + hints
		details.Notes = append(details.Notes, hints)
	}
	content := []ai.ToolResultMessageContent{ai.TextContent{Text: text}}
	if supportsImages {
		content = append(content, ai.ImageContent{MimeType: mime, Data: base64.StdEncoding.EncodeToString(data)})
	} else {
		text += "\n[Current model does not support images. The image will be omitted from this request.]"
		content = []ai.ToolResultMessageContent{ai.TextContent{Text: text}}
	}
	return agent.AgentToolResult{Content: content, Details: details}
}

// shownWindow is the 1-based line window of the document the model actually sees:
// the fetch's selected start, and as many lines as the output truncation kept.
func shownWindow(result webtools.FetchResult, truncation tools.TruncationResult) (window struct{ start, end int }) {
	window.start = max(result.StartLine, 1)
	if result.TotalLines == 0 {
		return window
	}
	lines := max(truncation.OutputLines, 1)
	window.end = min(window.start+lines-1, result.TotalLines)
	return window
}

// headerLine is the one-line preamble every result starts with.
func headerLine(result webtools.FetchResult) string {
	parts := []string{result.Method}
	if result.MediaType != "" {
		parts = append(parts, result.MediaType)
	}
	if result.TotalLines > 0 {
		parts = append(parts, strconv.Itoa(result.TotalLines)+" lines")
	}
	header := "Fetched " + result.URL + " (" + strings.Join(parts, ", ") + ")"
	if result.Title != "" {
		header += "\nTitle: " + result.Title
	}
	return header
}

// errorResult is a runtime failure: the transcript keeps the detail.
func errorResult(message string, details *Details) extension.AgentToolResult {
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: message}},
		Details: details,
		IsError: true,
	}
}

// floatToInt clamps a JSON number to a non-negative int.
func floatToInt(value float64) int {
	if value <= 0 || math.IsNaN(value) {
		return 0
	}
	if value > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(value)
}

// init registers the built-in definition, as importing core/tools gives upstream
// the definition: a session with --no-extensions still has web_fetch.
func init() {
	tools.RegisterExtendedBuiltinTool(ToolName, func(options tools.ExtendedBuiltinOptions) tools.ExtendedBuiltinTool {
		return tools.ExtendedBuiltinTool{Definition: Definition(Options{GetSettings: options.Settings})}
	})
}

// ensure the HTTP client type is the one the shared loader expects.
var _ *http.Client = webtools.NewHTTPClient()
