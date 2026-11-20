package tools

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/jsstring"
	"github.com/MichaelKinsy/PiG/internal/nodespawn"
)

// ─── Grep Tool ────────────────────────────────────────────────────────────────

type grepParams struct {
	Pattern    string   `json:"pattern"`
	Path       string   `json:"path,omitempty"`
	Glob       string   `json:"glob,omitempty"`       // pattern relative to path (e.g. '*.ts' or 'src/**/*.ts')
	IgnoreCase bool     `json:"ignoreCase,omitempty"` // case-insensitive search
	Literal    bool     `json:"literal,omitempty"`    // treat pattern as literal string (--fixed-strings)
	Context    *float64 `json:"context,omitempty"`    // lines of context around each match
	Limit      *float64 `json:"limit,omitempty"`      // max matches (default: 100)
}

// GrepTool runs ripgrep to search files, mirroring upstream grep.ts.
//
// RgPath, when non-empty, pins the ripgrep binary. Otherwise rg is resolved
// on every call through Tools (upstream ensureTool: <agentDir>/bin, PATH,
// then a download); with no Tools, PATH only.
type GrepTool struct {
	CWD    string
	RgPath string
	Tools  *ToolsManager
	// Operations delegates the filesystem reads; nil selects the local filesystem (upstream options.operations).
	Operations *GrepOperations
}

// GrepOperations provides the filesystem check used by grep.
type GrepOperations struct {
	// IsDirectory reports whether the path is a directory and fails when it does not exist.
	IsDirectory func(absolutePath string) (bool, error)
	// ReadFile is retained for source compatibility; context comes from ripgrep events.
	ReadFile func(absolutePath string) (string, error)
}

func (t *GrepTool) operations() GrepOperations {
	if t.Operations != nil {
		return *t.Operations
	}
	return GrepOperations{
		IsDirectory: func(path string) (bool, error) {
			info, err := os.Stat(path)
			if err != nil {
				return false, err
			}
			return info.IsDir(), nil
		},
	}
}

func (t *GrepTool) Name() string  { return "grep" }
func (t *GrepTool) Label() string { return "" }

func (t *GrepTool) Schema() ai.ToolSchema {
	return toolSchemaWithParameters(ai.ToolSchema{
		Name:        "grep",
		Description: "Search file contents for a pattern. Returns matching lines with file paths and line numbers. Respects .gitignore. Output is truncated to 100 matches or 50KB (whichever is hit first). Long lines are truncated to 500 chars. Patterns are regex by default; use literal=true to search source punctuation as plain text.",
	}, `{"type":"object","required":["pattern"],"properties":{
		"pattern":{"type":"string","description":"Search pattern (regex or literal string)"},
		"path":{"type":"string","description":"Directory or file to search (default: current directory)"},
		"glob":{"type":"string","description":"Filter files by glob pattern relative to path, e.g. 'src/**/*.ts'. Without '/', '*.ts' matches file names at any depth"},
		"ignoreCase":{"type":"boolean","description":"Case-insensitive search (default: false)"},
		"literal":{"type":"boolean","description":"Treat pattern as literal string instead of regex (default: false)"},
		"context":{"type":"number","description":"Number of lines to show before and after each match (default: 0)"},
		"limit":{"type":"number","description":"Maximum number of matches to return (default: 100)"}
	}}`)
}

func (t *GrepTool) ExecutionMode() agent.ToolExecutionMode { return agent.ToolModeParallel }

// grepDefaultLimit mirrors upstream DEFAULT_LIMIT.
const grepDefaultLimit = 100

// grepMatch is one rg "match" or "context" event.
type grepMatch struct {
	filePath   string
	lineNumber int
	lineText   *string
	isContext  bool
}

// Execute resolves the search path, consumes ripgrep's match and context events,
// and formats the output with upstream's truncation and limit notices.
func (t *GrepTool) Execute(ctx context.Context, _ string, rawParams json.RawMessage, _ agent.ToolUpdateCallback) (agent.AgentToolResult, error) {
	var p grepParams
	if err := json.Unmarshal(rawParams, &p); err != nil {
		return agent.AgentToolResult{}, fmt.Errorf("grep: invalid params: %w", err)
	}
	if ctx.Err() != nil {
		return grepError("Operation aborted"), nil
	}
	rgPath := ensureSearchTool(ctx, t.RgPath, t.Tools, "rg")
	if rgPath == "" {
		return grepError("ripgrep (rg) is not available and could not be downloaded"), nil
	}

	searchDir := p.Path
	if searchDir == "" {
		searchDir = "."
	}
	cwd, err := toolCWD(ctx, t.CWD)
	if err != nil {
		return agent.AgentToolResult{}, err
	}
	searchPath, err := resolvePath(cwd, searchDir)
	if err != nil {
		return agent.AgentToolResult{}, err
	}
	ops := t.operations()
	isDirectory, err := ops.IsDirectory(searchPath)
	if err != nil {
		return grepError("Path not found: " + searchPath), nil
	}

	contextValue := 0.0
	if p.Context != nil && *p.Context > 0 {
		contextValue = math.Min(math.Floor(*p.Context), 1<<53-1)
	}
	effectiveLimit := float64(grepDefaultLimit)
	if p.Limit != nil {
		effectiveLimit = *p.Limit
	}
	effectiveLimit = math.Max(1, math.Floor(effectiveLimit))

	// --no-config: the tool contract (literal, glob, limit, ...) must not depend
	// on ambient user config like RIPGREP_CONFIG_PATH.
	args := []string{"--no-config", "--json", "--line-number", "--color=never", "--hidden"}
	if p.IgnoreCase {
		args = append(args, "--ignore-case")
	}
	if p.Literal {
		args = append(args, "--fixed-strings")
	}
	if p.Glob != "" {
		args = append(args, "--glob", normalizeGrepGlob(p.Glob))
	}
	if contextValue > 0 {
		args = append(args, "--context", jsNumber(contextValue))
	}
	searchTarget := searchPath
	commandDir := cwd
	if isDirectory {
		searchTarget = "."
		commandDir = searchPath
	}
	args = append(args, "--", p.Pattern, searchTarget)

	f := grepFormatter{searchPath: searchPath, isDirectory: isDirectory}
	joined := newHeadJoin(truncationLimits(DefaultMaxBytes, 1<<53-1))
	run, err := runRipgrep(ctx, rgPath, commandDir, args, effectiveLimit, contextValue, func(m grepMatch) {
		f.emitEvent(m, joined)
	})
	if err != nil {
		// Upstream's catch rejects with an error spawn throws unchanged; only
		// the child's error event adds the prefix.
		if spawnErr, ok := errors.AsType[*nodespawn.Error](err); ok && spawnErr.Thrown {
			return grepError(err.Error()), nil
		}
		return grepError("Failed to run ripgrep: " + err.Error()), nil
	}
	if ctx.Err() != nil {
		return grepError("Operation aborted"), nil
	}
	if !run.killedDueToLimit && run.exitCode != 0 && run.exitCode != 1 {
		msg := jsstring.Trim(run.stderr)
		if msg == "" {
			msg = fmt.Sprintf("ripgrep exited with code %d", run.exitCode)
		}
		if !p.Literal && strings.Contains(msg, "regex parse error:") {
			msg = TruncateTail(msg, TruncationOptions{MaxBytes: new(2048), MaxLines: new(8)}).Content +
				"\nIf you intended plain text, retry with literal=true. Otherwise correct the regex."
		}
		return grepError(msg), nil
	}
	if run.matchCount == 0 {
		return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: "No matches found"}}}, nil
	}

	tr := joined.Result()
	output := tr.Content
	var notices []string
	details := &GrepDetails{}
	if run.matchLimitReached {
		notices = append(notices, fmt.Sprintf("%s matches limit reached. Use limit=%s for more, or refine pattern",
			jsNumber(effectiveLimit), jsNumber(effectiveLimit*2)))
		details.MatchLimitReached = effectiveLimit
	}
	if tr.Truncated {
		notices = append(notices, FormatSize(DefaultMaxBytes)+" limit reached")
		trc := tr
		details.Truncation = &trc
	}
	if f.linesTruncated {
		notices = append(notices, fmt.Sprintf("Some lines truncated to %d chars. Use read tool to see full lines", GrepMaxLineLength))
		details.LinesTruncated = true
	}
	if len(notices) > 0 {
		output += "\n\n[" + strings.Join(notices, ". ") + "]"
		shareTruncatedContent(details.Truncation, output)
	}
	result := agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: output}}}
	if len(notices) > 0 {
		result.Details = details
	}
	return result, nil
}

func grepError(message string) agent.AgentToolResult {
	return agent.AgentToolResult{Content: []ai.ToolResultMessageContent{ai.TextContent{Text: message}}, Details: map[string]any{}, IsError: true, Thrown: true}
}

// ripgrepRun is the outcome of one rg process.
type ripgrepRun struct {
	matchCount        int
	matchLimitReached bool
	killedDueToLimit  bool
	exitCode          int
	stderr            string
}

// runRipgrep consumes rg JSON events and stops only after observing a match
// beyond the user's limit. It keeps trailing context for the last included
// match, while context itself never consumes the match limit.
func runRipgrep(ctx context.Context, rgPath, dir string, args []string, limit, contextValue float64, emit func(grepMatch)) (ripgrepRun, error) {
	//nolint:gosec // G204 (class b): program is rg resolved by PiG's tools manager; args are fixed flags, then -- and the pattern and search path from the grep tool call, so the model cannot reach an rg option.
	cmd := exec.CommandContext(ctx, rgPath, args...)
	// Upstream's spawn has no cwd option, so on Windows libuv finds rg from
	// PiG's working directory: the lookup precedes cmd.Dir.
	nodespawn.SetProgram(cmd)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ripgrepRun{}, err
	}
	if err := cmd.Start(); err != nil {
		return ripgrepRun{}, err
	}
	var run ripgrepRun
	var lastFile string
	var trailingContextEnd float64
	limitHit := false
	stopped := false
	reader := bufio.NewReader(stdout)
	push := func(m grepMatch, isMatch bool) {
		if limitHit {
			inWindow := m.filePath == lastFile && float64(m.lineNumber) <= trailingContextEnd
			if inWindow {
				m.isContext = true
				emit(m)
				lastFile = m.filePath
			}
			if isMatch {
				run.matchLimitReached = true
			}
			if run.matchLimitReached && !(inWindow && float64(m.lineNumber) < trailingContextEnd) {
				run.killedDueToLimit = true
				stopped = true
				_ = cmd.Process.Kill()
			}
			return
		}
		emit(m)
		lastFile = m.filePath
		if !isMatch {
			return
		}
		run.matchCount++
		if float64(run.matchCount) >= limit {
			limitHit = true
			trailingContextEnd = float64(m.lineNumber) + contextValue
		}
	}
	for {
		line, readErr := reader.ReadString('\n')
		if !stopped && jsstring.Trim(line) != "" {
			m, isMatch := parseRipgrepEvent(line)
			if m != nil {
				if !filepath.IsAbs(m.filePath) {
					m.filePath = filepath.Join(dir, filepath.FromSlash(m.filePath))
				}
				push(*m, isMatch)
			}
		}
		if readErr != nil {
			break
		}
	}
	waitErr := cmd.Wait()
	run.stderr = stderr.String()
	if exitErr, ok := errors.AsType[*exec.ExitError](waitErr); ok {
		run.exitCode = exitErr.ExitCode()
	} else if waitErr != nil && !run.killedDueToLimit && ctx.Err() == nil {
		return run, waitErr
	}
	return run, nil
}

type rgJSONText struct {
	Text  *string `json:"text"`
	Bytes *string `json:"bytes"`
}

// parseRipgrepEvent parses a match or context event with its path and line text.
func parseRipgrepEvent(line string) (*grepMatch, bool) {
	var event struct {
		Type string `json:"type"`
		Data struct {
			Path       *rgJSONText `json:"path"`
			LineNumber *float64    `json:"line_number"`
			Lines      *rgJSONText `json:"lines"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(line), &event); err != nil || (event.Type != "match" && event.Type != "context") {
		return nil, false
	}
	isMatch := event.Type == "match"
	d := event.Data
	filePath := decodeRgJSONText(d.Path)
	if filePath == "" || d.LineNumber == nil {
		return nil, false
	}
	//portlint:allow numbers ripgrep reports line numbers as positive integers within the range of int
	m := &grepMatch{filePath: filePath, lineNumber: int(*d.LineNumber), isContext: !isMatch}
	if d.Lines != nil {
		lineText := decodeRgJSONText(d.Lines)
		m.lineText = &lineText
	}
	return m, isMatch
}

func decodeRgJSONText(data *rgJSONText) string {
	if data == nil {
		return ""
	}
	if data.Text != nil {
		return *data.Text
	}
	if data.Bytes != nil {
		decoded, err := base64.StdEncoding.DecodeString(*data.Bytes)
		if err == nil {
			return jsstring.FromUTF8(decoded)
		}
	}
	return ""
}

func normalizeGrepGlob(glob string) string {
	negation := ""
	if strings.HasPrefix(glob, "!") {
		negation, glob = "!", glob[1:]
	}
	for strings.HasPrefix(glob, "./") {
		glob = strings.TrimPrefix(glob, "./")
	}
	return negation + glob
}

// grepFormatter formats rg's match and context events.
type grepFormatter struct {
	searchPath     string
	isDirectory    bool
	linesTruncated bool
}

func (f *grepFormatter) formatPath(filePath string) string {
	if f.isDirectory {
		if rel, err := filepath.Rel(f.searchPath, filePath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.Base(filePath)
}

func (f *grepFormatter) truncate(line string) string {
	truncated := TruncateLine(line, GrepMaxLineLength)
	text, wasTruncated := truncated.Text, truncated.WasTruncated
	if wasTruncated {
		f.linesTruncated = true
	}
	return text
}

// emitEvent adds one rg event to out. Once out holds every byte its result
// can show, the line is only measured so output memory stays bounded.
func (f *grepFormatter) emitEvent(m grepMatch, out *headJoin) {
	relativePath := f.formatPath(m.filePath)
	lineText := ""
	if m.lineText != nil {
		lineText = *m.lineText
		if strings.HasSuffix(lineText, "\r\n") {
			lineText = strings.TrimSuffix(lineText, "\r\n")
		} else {
			lineText = strings.TrimSuffix(lineText, "\n")
		}
	}
	separator := ":"
	if m.isContext {
		separator = "-"
	}
	prefix := fmt.Sprintf("%s%s%d%s ", relativePath, separator, m.lineNumber, separator)
	if out.full() && !strings.Contains(lineText, "\n") {
		out.AddLength(len(prefix) + f.truncatedBytes(lineText))
		return
	}
	out.Add(prefix + f.truncate(lineText))
}

// truncatedBytes is len(f.truncate(line)), recording a cut line as f.truncate does.
func (f *grepFormatter) truncatedBytes(line string) int {
	n, wasTruncated := truncatedLineBytes(line, GrepMaxLineLength)
	if wasTruncated {
		f.linesTruncated = true
	}
	return n
}

// GrepMaxLineLength matches upstream truncate.ts GREP_MAX_LINE_LENGTH.
const GrepMaxLineLength = GrepMaxLineLengthUpstream
