package codingagent

// Auto-read of `@path` mentions in a submitted prompt.
//
// Ports oh-my-pi's packages/coding-agent/src/utils/file-mentions.ts. When a
// prompt mentions a file or directory, the mention is read here and delivered
// to the model beside the prompt, so the agent does not spend a turn calling
// read. The transcript shows what was read under the submitted prompt.
//
// Divergences from the upstream port, both forced by this runtime:
//   - Only text output is produced. PiG has no video contact sheets or markit
//     converters, so image files report their attachment in the content and are
//     not decoded into image blocks here.
//   - Reading goes through the read tool's own resolver and truncation so a
//     mention and a later `read` call report identical text.

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/internal/lazyregexp"
)

// fileMentionMaxAutoReadBytes bounds a mentioned file's auto-read. A larger file
// is reported by size instead of being read into the prompt, as upstream does.
const fileMentionMaxAutoReadBytes = 5 * 1024 * 1024

// fileMentionDirEntryLimit caps a mentioned directory's listing. Upstream's
// DEFAULT_DIR_LIMIT is 500 entries.
const fileMentionDirEntryLimit = 500

// fileMentionRegex matches `@path`, `@"quoted path"` and `@'quoted path'`.
// Group 1 and 2 are the quoted forms, group 3 the bare token.
var fileMentionRegex = lazyregexp.New(`@(?:"([^"]+)"|'([^']+)'|([^\s@]+))`)

// fileMentionLeadingPunct and fileMentionTrailingPunct are stripped from a bare
// mention, so `(@src/a.ts),` resolves to `src/a.ts`.
const (
	fileMentionLeadingPunct  = "`\"'([{<"
	fileMentionTrailingPunct = ")]}>.,;:!?\"'`"
)

// FileMention is one mentioned path and the text auto-read produced for it.
// The transcript card renders these rows; the provider receives the same
// content as a developer message. The JSON tags are the session entry's
// `details` shape, which is how a resumed transcript rebuilds the card.
type FileMention struct {
	// Path is the mention as the user wrote it, without the `@`.
	Path string `json:"path"`
	// Content is the auto-read text: the truncated file, the directory
	// listing, or the reason the read was skipped.
	Content string `json:"content"`
	// LineCount is the number of lines Content holds for a file, or the
	// directory entries for a directory listing. Zero for a skipped read.
	LineCount int `json:"lineCount,omitempty"`
	// ByteSize is the mentioned file's size when it was skipped as too large
	// or binary, so the card can name it.
	ByteSize int64 `json:"byteSize,omitempty"`
	// IsDirectory marks a directory listing rather than file content.
	IsDirectory bool `json:"isDirectory,omitempty"`
	// Skipped is the reason the content was not read, or "" when it was.
	Skipped string `json:"skippedReason,omitempty"`
}

// fileMentionIsBoundary reports whether a mention may start at index: upstream
// requires a start-of-text or whitespace/opening-punctuation boundary so
// `user@host` is prose.
func fileMentionIsBoundary(text string, index int) bool {
	if index == 0 {
		return true
	}
	switch text[index-1] {
	case ' ', '\t', '\n', '\r', '(', '[', '{', '<', '"', '\'', '`':
		return true
	}
	return false
}

// fileMentionPath cleans a bare mention token: leading and trailing punctuation
// around a path is prose, not part of the name.
func fileMentionPath(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimLeft(raw, fileMentionLeadingPunct)
	raw = strings.TrimRight(raw, fileMentionTrailingPunct)
	return strings.TrimSpace(raw)
}

// ExtractFileMentions returns the distinct paths a prompt mentions, in first
// appearance order, including quoted mentions with spaces. It never fails: a
// mention that is not an existing path is prose and is dropped later by
// ReadFileMentions.
func ExtractFileMentions(text string) []string {
	mentions := make([]string, 0, 4)
	seen := make(map[string]bool)
	for _, match := range fileMentionRegex.FindAllStringSubmatchIndex(text, -1) {
		if !fileMentionIsBoundary(text, match[0]) {
			continue
		}
		var path string
		switch {
		case match[2] >= 0:
			path = strings.TrimSpace(text[match[2]:match[3]])
		case match[4] >= 0:
			path = strings.TrimSpace(text[match[4]:match[5]])
		case match[6] >= 0:
			path = fileMentionPath(text[match[6]:match[7]])
		}
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		mentions = append(mentions, path)
	}
	return mentions
}

// fileMentionText truncates a mentioned file exactly as the read tool would,
// so the auto-read and a later read call report the same text.
func fileMentionText(content string) (string, int) {
	result := tools.TruncateHead(content, tools.TruncationOptions{})
	lines := strings.Count(content, "\n") + 1
	output := result.Content
	if result.Truncated {
		if warning := tools.FormatTruncationWarning(result); warning != "" {
			output += "\n\n" + warning
		}
	}
	return output, lines
}

// fileMentionDirectory lists a mentioned directory the way the read tool lists
// one (directories first, `name/`), one entry per line and capped at
// fileMentionDirEntryLimit.
func fileMentionDirectory(path string) (string, int, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", 0, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		dirI, dirJ := strings.HasSuffix(names[i], "/"), strings.HasSuffix(names[j], "/")
		if dirI != dirJ {
			return dirI
		}
		return names[i] < names[j]
	})
	if len(names) == 0 {
		// Zero entries, so the card can say "empty" instead of "0 entries".
		return "(empty directory)", 0, nil
	}
	count := len(names)
	if count > fileMentionDirEntryLimit {
		names = names[:fileMentionDirEntryLimit]
	}
	output, _ := fileMentionText(strings.Join(names, "\n"))
	if count > fileMentionDirEntryLimit {
		output += fmt.Sprintf("\n\n[%d entries limit reached. Use read with limit=%d for more]", fileMentionDirEntryLimit, fileMentionDirEntryLimit*2)
	}
	return output, count, nil
}

// ReadFileMentions resolves each mention against cwd and auto-reads it. A
// mention that does not resolve to an existing path is skipped silently, as
// upstream does: the `@` selector inserts a real path, so anything else is
// prose. The returned slice is nil when nothing could be read.
func ReadFileMentions(mentions []string, cwd string) []FileMention {
	read := make([]FileMention, 0, len(mentions))
	for _, mention := range mentions {
		absolute, err := tools.ResolveReadPath(mention, cwd)
		if err != nil {
			continue
		}
		info, err := os.Stat(absolute)
		if err != nil {
			continue
		}
		if info.IsDir() {
			content, count, err := fileMentionDirectory(absolute)
			if err != nil {
				continue
			}
			read = append(read, FileMention{Path: mention, Content: content, LineCount: count, IsDirectory: true})
			continue
		}
		if info.Size() > fileMentionMaxAutoReadBytes {
			read = append(read, FileMention{
				Path:     mention,
				Content:  "(skipped auto-read: too large, " + tools.FormatSize(int(info.Size())) + ")",
				ByteSize: info.Size(),
				Skipped:  "tooLarge",
			})
			continue
		}
		data, err := os.ReadFile(absolute)
		if err != nil {
			continue
		}
		if isProbablyBinary(data) {
			read = append(read, FileMention{
				Path:     mention,
				Content:  "(skipped auto-read: binary file, " + tools.FormatSize(len(data)) + ")",
				ByteSize: info.Size(),
				Skipped:  "binary",
			})
			continue
		}
		content, lines := fileMentionText(string(data))
		read = append(read, FileMention{Path: mention, Content: content, LineCount: lines})
	}
	if len(read) == 0 {
		return nil
	}
	return read
}

// isProbablyBinary reports whether data holds a NUL byte in its first 8KB,
// which is how git and OMP separate text from binary without a MIME table.
func isProbablyBinary(data []byte) bool {
	window := data
	if len(window) > 8192 {
		window = window[:8192]
	}
	for _, b := range window {
		if b == 0 {
			return true
		}
	}
	return false
}

// FileMentionsText is the developer message the model receives for the
// mentions: one `<file path="...">` block per mention, in prompt order.
// It returns "" when nothing was read.
func FileMentionsText(mentions []FileMention) string {
	if len(mentions) == 0 {
		return ""
	}
	var builder strings.Builder
	for i, mention := range mentions {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(`<file path="`)
		builder.WriteString(mention.Path)
		builder.WriteString(`">`)
		builder.WriteString("\n")
		builder.WriteString(mention.Content)
		builder.WriteString("\n</file>")
	}
	return builder.String()
}

// FileMentionsCustomType is the custom message type that carries an auto-read.
// Its content is the model-visible text; its details are the rows the
// transcript card draws, so a resumed session rebuilds the same tree.
const FileMentionsCustomType = "file-mentions"

// FileMentionsMessage builds the custom message that delivers mentions beside
// the prompt that mentioned them. displays is always true: the card is the
// point of the message. The returned message is ready for Agent.QueueNextTurn,
// Steer or FollowUp.
func FileMentionsMessage(mentions []FileMention) agent.AgentMessage {
	return agent.AgentMessage{Custom: map[string]any{
		"role":       agent.RoleCustom,
		"customType": FileMentionsCustomType,
		"content":    FileMentionsText(mentions),
		"display":    true,
		"details":    map[string]any{"mentions": mentions},
		"timestamp":  time.Now().UnixMilli(),
	}}
}

// FileMentionsOf decodes the mentions an auto-read message or session entry
// carries. Details arrive as []FileMention for a message this process built and
// as decoded JSON for one loaded from a session file, so both are normalized.
func FileMentionsOf(details any) []FileMention {
	switch details := details.(type) {
	case map[string]any:
		return fileMentionsFromJSON(details["mentions"])
	case []FileMention:
		return details
	}
	return fileMentionsFromJSON(details)
}

// fileMentionsFromJSON decodes a mentions value that reached this process as
// JSON: a []any of objects, or their encoded form.
func fileMentionsFromJSON(value any) []FileMention {
	if value == nil {
		return nil
	}
	if mentions, ok := value.([]FileMention); ok {
		return mentions
	}
	encoded, err := jsstring.MarshalJSON(value)
	if err != nil {
		return nil
	}
	var wrapper struct {
		Mentions []FileMention `json:"mentions"`
	}
	if json.Unmarshal(encoded, &wrapper) == nil && len(wrapper.Mentions) > 0 {
		return wrapper.Mentions
	}
	var mentions []FileMention
	if json.Unmarshal(encoded, &mentions) != nil {
		return nil
	}
	return mentions
}
