package tools

// read.go's URL branch: `read` covers what the retired `web_fetch` tool did.
// OMP has no web_fetch tool — its read tool accepts a URL path and renders it
// through the same pipeline (packages/tui/src/tools/read.ts isReadableUrlPath,
// tools/fetch.ts). Porting that means one tool name and one card for both a
// file and a document.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/outputfiles"
	"github.com/MichaelKinsy/PiG/internal/webtools"
)

// Tool-boundary output limits for a URL read: the model sees the head of the
// document and the remainder is written to an artifact `read` can page through.
// These are the retired web_fetch tool's limits, kept so existing prompts and
// continuation notes keep working.
const (
	readURLMaxOutputLines = 300
	readURLMaxOutputBytes = 50 * 1024
	// readURLTimeout bounds one whole fetch. The read schema exposes no timeout,
	// so it matches the fetch pipeline's default.
	readURLTimeout = webtools.DefaultFetchTimeout
)

// isReadableURLPath is read.ts isReadableUrlPath: a bare scheme or a `www.`
// host. A path that is not one stays a filesystem path.
func isReadableURLPath(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "www.")
}

// readURL executes the URL branch of `read`, returning ok=false for a
// filesystem path. It mirrors the retired web_fetch tool's rendering exactly:
// the same header line, truncation artifact, continuation range and notes,
// because the model-facing contract does not change when the tool name does.
func (t *ReadTool) readURL(ctx context.Context, p readParams) (agent.AgentToolResult, bool, error) {
	if !isReadableURLPath(p.Path) {
		return agent.AgentToolResult{}, false, nil
	}
	if ctx.Err() != nil {
		return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "Operation aborted"}}, Details: map[string]any{}, IsError: true, Thrown: true}, true, nil
	}
	options := webtools.FetchOptions{Reader: "native", Timeout: readURLTimeout}
	if p.Offset != nil && *p.Offset > 0 {
		options.Offset = clampPositiveInt(*p.Offset)
	}
	if p.Limit != nil && *p.Limit > 0 {
		options.Limit = clampPositiveInt(*p.Limit)
	}
	result, err := webtools.Fetch(ctx, p.Path, options)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "Operation aborted"}}, Details: map[string]any{}, IsError: true, Thrown: true}, true, nil
		}
		return readURLError("Error: " + err.Error()), true, nil
	}
	return renderReadURL(ctx, result), true, nil
}

// readDirectory is read.ts #readDirectory: a directory reads as a sorted
// listing, directories first (`name/`) then files, or `(empty directory)`.
// Entries named by URL in OMP's internal-resource handlers are listed by name
// here, which is what the model and the transcript both show.
func (t *ReadTool) readDirectory(path string, p readParams) (agent.AgentToolResult, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return agent.AgentToolResult{}, false
	}
	entries, readErr := os.ReadDir(path)
	if readErr != nil {
		return agent.AgentToolResult{
			Content: []ai.ToolResultMessageContent{ai.TextContent{Text: NodeFSError(readErr, "readdir", path)}},
			Details: &ReadDetails{Path: path, IsDirectory: true},
			IsError: true,
		}, true
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	// Directories first, then files, each by name: formatDirectoryListing.
	sort.SliceStable(names, func(i, j int) bool {
		dirI, dirJ := strings.HasSuffix(names[i], "/"), strings.HasSuffix(names[j], "/")
		if dirI != dirJ {
			return dirI
		}
		return names[i] < names[j]
	})
	listing := "(empty directory)"
	if len(names) > 0 {
		listing = strings.Join(names, "\n")
	}
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: listing}},
		Details: &ReadDetails{Path: path, IsDirectory: true},
	}, true
}

// clampPositiveInt bounds a JSON number to [1, MaxInt32], matching the retired
// tool's floatToInt.
func clampPositiveInt(value float64) int {
	if math.IsNaN(value) || value > math.MaxInt32 {
		return math.MaxInt32
	}
	if value < 1 {
		return 1
	}
	return int(value)
}

// readURLDetails is the metadata a URL read records. It is the retired
// web_fetch tool's Details shape, so a session replayed from an older file
// still renders.
type readURLDetails struct {
	URL            string   `json:"url"`
	Method         string   `json:"method"`
	MediaType      string   `json:"mediaType,omitempty"`
	Title          string   `json:"title,omitempty"`
	Status         int      `json:"status,omitempty"`
	Notes          []string `json:"notes,omitempty"`
	TotalLines     int      `json:"totalLines,omitempty"`
	StartLine      int      `json:"startLine,omitempty"`
	EndLine        int      `json:"endLine,omitempty"`
	Truncated      bool     `json:"truncated,omitempty"`
	FullOutputPath string   `json:"fullOutputPath,omitempty"`
	// Kind marks the read as a URL so the card renders it as a document.
	Kind string `json:"kind"`
}

// renderReadURL turns a fetch into the read result: an image attachment when
// the URL is an image the model accepts, otherwise truncated text plus an
// artifact and the continuation notices.
func renderReadURL(ctx context.Context, result webtools.FetchResult) agent.AgentToolResult {
	details := &readURLDetails{
		URL:        result.URL,
		Method:     result.Method,
		MediaType:  result.MediaType,
		Title:      result.Title,
		Status:     result.Status,
		Notes:      append([]string(nil), result.Notes...),
		TotalLines: result.TotalLines,
		StartLine:  result.StartLine,
		EndLine:    result.EndLine,
		Truncated:  result.Truncated,
		Kind:       "url",
	}
	if result.Image != nil {
		return renderReadURLImage(ctx, result, details)
	}
	header := readURLHeader(result)
	body := result.Text
	if strings.TrimSpace(body) == "" {
		if result.TotalLines == 0 {
			body = "(the document rendered to no text)"
		} else {
			body = fmt.Sprintf("(the requested line window is empty; the document has %d lines)", result.TotalLines)
		}
	}
	truncation := TruncateHead(body, TruncationOptions{MaxLines: new(readURLMaxOutputLines), MaxBytes: new(readURLMaxOutputBytes)})
	visible := truncation.Content
	var tail []string
	if warning := FormatTruncationWarning(truncation); warning != "" {
		details.Truncated = true
		if path, err := outputfiles.WriteFile("pi-fetch", ".md", []byte(body)); err == nil {
			details.FullOutputPath = path
			tail = append(tail, fmt.Sprintf("%s Full output: %s (read it with offset/limit).", warning, path))
		} else {
			tail = append(tail, warning)
		}
	} else if truncation.FirstLineExceedsLimit {
		details.Truncated = true
		tail = append(tail, FormatTruncationWarning(truncation))
	}
	if shown := readURLShownWindow(result, truncation); shown.start <= shown.end && (shown.start > 1 || shown.end < result.TotalLines) {
		note := fmt.Sprintf("[Showing lines %d-%d of %d.", shown.start, shown.end, result.TotalLines)
		if shown.end < result.TotalLines {
			note += fmt.Sprintf(" Append :%d-%d to the URL for the next range.", shown.end+1, min(shown.end+readURLMaxOutputLines, result.TotalLines))
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
	return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: text}}, Details: details}
}

// renderReadURLImage returns the image as an attachment when the model accepts
// images, and describes it otherwise.
func renderReadURLImage(ctx context.Context, result webtools.FetchResult, details *readURLDetails) agent.AgentToolResult {
	env, _ := agent.ToolEnvironmentFrom(ctx)
	supportsImages := env.SupportsImages == nil || *env.SupportsImages
	data, mime, hints, err := processReadImage(result.Image.Data, result.Image.MIME, supportsImages, nil)
	text := readURLHeader(result)
	if err != nil {
		text += "\n\n" + err.Error()
		return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: text}}, Details: details}
	}
	text += "\n\n[Image " + mime + ", " + FormatSize(len(data)) + "]"
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

// readURLShownWindow is the 1-based window of the document the model actually
// sees: the fetch's selected start and as many lines as truncation kept.
func readURLShownWindow(result webtools.FetchResult, truncation TruncationResult) (window struct{ start, end int }) {
	window.start = max(result.StartLine, 1)
	if result.TotalLines == 0 {
		return window
	}
	lines := max(truncation.OutputLines, 1)
	window.end = min(window.start+lines-1, result.TotalLines)
	return window
}

// readURLHeader is the one-line preamble every URL read starts with.
func readURLHeader(result webtools.FetchResult) string {
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

// readURLError is a runtime failure: the transcript keeps the detail.
func readURLError(message string) agent.AgentToolResult {
	return agent.AgentToolResult{
		Content: []ai.ToolResultMessageContent{ai.TextContent{Text: message}},
		Details: &readURLDetails{},
		IsError: true,
	}
}

// ReadURLDetailsOf reports whether a read result came from a URL and returns
// its metadata. The coding agent package cannot import this one's unexported
// types, so it asks through this predicate.
func ReadURLDetailsOf(details any) (*ReadURLDetails, bool) {
	d, ok := readURLDetailsOf(details)
	return (*ReadURLDetails)(d), ok
}

// ReadURLDetails is the exported view of a URL read's metadata.
type ReadURLDetails = readURLDetails

// readURLDetailsOf reads the URL metadata from a result's details, whether it
// arrives typed or as a replayed JSON object.
func readURLDetailsOf(details any) (*readURLDetails, bool) {
	switch value := details.(type) {
	case *readURLDetails:
		return value, value != nil
	case readURLDetails:
		return &value, true
	case map[string]any:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}
		var out readURLDetails
		if json.Unmarshal(encoded, &out) != nil || out.Kind != "url" {
			return nil, false
		}
		return &out, true
	}
	return nil, false
}
