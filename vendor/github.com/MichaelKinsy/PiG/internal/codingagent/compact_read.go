package codingagent

import (
	"path/filepath"

	"github.com/MichaelKinsy/PiG/internal/codingagent/tools"
	"github.com/MichaelKinsy/PiG/tui"
)

// compactResourceFileNames is upstream read.ts COMPACT_RESOURCE_FILE_NAMES.
var compactResourceFileNames = map[string]bool{"AGENTS.override.md": true, "AGENTS.md": true, "AGENTS.MD": true, "CLAUDE.md": true, "CLAUDE.MD": true}

func init() {
	tui.SetCompactReadClassifier(classifyCompactRead)
}

// classifyCompactRead is upstream read.ts getCompactReadClassification: a
// SKILL.md reads as "[skill] <directory>", the agent's own documentation as
// "read docs <path>", and a context file as "read resource <path>".
func classifyCompactRead(rawPath, cwd string) (tui.CompactReadClassification, bool) {
	absolutePath, err := tools.ResolveToCwd(rawPath, cwd)
	if err != nil {
		return tui.CompactReadClassification{}, false
	}
	fileName := filepath.Base(absolutePath)
	if fileName == "SKILL.md" {
		label := filepath.Base(filepath.Dir(absolutePath))
		if label == "" || label == "." || label == string(filepath.Separator) {
			label = fileName
		}
		return tui.CompactReadClassification{Kind: "skill", Label: label}, true
	}
	if compactResourceFileNames[fileName] {
		return tui.CompactReadClassification{Kind: "resource", Label: FormatPathRelativeToCwdOrAbsolute(absolutePath, cwd)}, true
	}
	return tui.CompactReadClassification{}, false
}
