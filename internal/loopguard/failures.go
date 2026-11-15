package loopguard

import (
	"crypto/sha256"
	"path/filepath"
	"strings"

	"github.com/MichaelKinsy/PiG/agent"
	"github.com/MichaelKinsy/PiG/ai"
)

const failureThreshold = 3

type failureFingerprint struct {
	tool, class string
	target      [sha256.Size]byte
}

type failureStreak struct {
	last  failureFingerprint
	count int
}

func deterministicClass(tool, text string) string {
	if firstLine, _, found := strings.Cut(text, "\n"); found {
		text = firstLine
	}
	switch tool {
	case "edit":
		if strings.HasPrefix(text, "Could not find ") &&
			(strings.HasSuffix(text, ". The old text must match exactly including all whitespace and newlines.") ||
				strings.HasSuffix(text, ". The oldText must match exactly including all whitespace and newlines.")) {
			return "edit-unmatched"
		}
		if strings.HasPrefix(text, "Found ") && strings.Contains(text, " occurrences of ") &&
			(strings.HasSuffix(text, ". The text must be unique. Please provide more context to make it unique.") ||
				strings.HasSuffix(text, ". Each oldText must be unique. Please provide more context to make it unique.")) {
			return "edit-ambiguous"
		}
	case "grep":
		if strings.HasPrefix(text, "regex parse error:") || strings.HasPrefix(text, "rg: regex parse error:") {
			return "grep-invalid-regex"
		}
	}
	return ""
}

func failureTarget(call ai.ToolCall) (string, bool) {
	if call.Arguments == nil {
		return "", false
	}
	path, ok := call.Arguments["path"].(string)
	if !ok && call.Arguments["path"] != nil {
		return "", false
	}
	if path == "" {
		if call.Name != "grep" {
			return "", false
		}
		path = "."
	}
	return filepath.Clean(path), true
}

func (s *failureStreak) record(calls []ai.ToolCall, results []agent.ToolResultMessage) *detection {
	byID := make(map[string]agent.ToolResultMessage, len(results))
	for _, result := range results {
		byID[result.ToolCallID] = result
	}
	var candidate failureFingerprint
	var diagnostic *detection
	var successes = make(map[[sha256.Size]byte]bool)
	uncertain := false
	observe := func(call ai.ToolCall, isError bool, text string) {
		target, hasTarget := failureTarget(call)
		if !isError {
			if hasTarget {
				successes[sha256.Sum256([]byte(target))] = true
			}
			return
		}
		class := deterministicClass(call.Name, text)
		if class == "" || !hasTarget {
			uncertain = true
			return
		}
		fingerprint := failureFingerprint{tool: call.Name, class: class, target: sha256.Sum256([]byte(target))}
		if diagnostic != nil && candidate != fingerprint {
			uncertain = true
		}
		candidate = fingerprint
		diagnostic = &detection{ToolName: call.Name, FailureClass: class, TargetSummary: summarize(target, 200)}
	}
	for _, call := range calls {
		result, ok := byID[call.ID]
		if !ok {
			uncertain = true
			continue
		}
		if call.Name == "codemode" {
			if result.NestedCalls == nil || !result.NestedCalls.Complete {
				uncertain = true
				continue
			}
			for _, nested := range result.NestedCalls.Calls {
				if nested.Arguments == nil || (nested.Status != ai.NestedToolCallOK && nested.Status != ai.NestedToolCallError) {
					uncertain = true
					continue
				}
				observe(ai.ToolCall{Name: nested.Name, Arguments: nested.Arguments}, nested.Status == ai.NestedToolCallError, nested.Error)
			}
			continue
		}
		var text strings.Builder
		if result.IsError {
			for _, block := range result.Content {
				if block, ok := block.(ai.TextContent); ok {
					text.WriteString(block.Text)
				}
			}
		}
		observe(call, result.IsError, text.String())
	}
	if uncertain || successes[s.last.target] || (diagnostic != nil && successes[candidate.target]) {
		*s = failureStreak{}
		return nil
	}
	if diagnostic == nil {
		return nil
	}
	if s.last == candidate {
		s.count = min(s.count+1, failureThreshold)
	} else {
		s.last, s.count = candidate, 1
	}
	if s.count < failureThreshold {
		return nil
	}
	diagnostic.Count = s.count
	return diagnostic
}
