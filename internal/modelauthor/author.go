// Package modelauthor attributes an agent's git commits to the model that wrote
// them, so the model appears as a contributor. It rewrites `git commit` in the
// bash tool instead of asking the model to remember a rule.
package modelauthor

import (
	"regexp"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
)

// Matching is by model ID, not provider. Only vendors with a confirmed GitHub
// bot or agent account appear; docs/model-author.md lists them and the vendors
// left out, with the reason.
var (
	codexFamily = regexp.MustCompile(`(?:^|[/.:])(?:gpt-|codex|o[134](?:-|$))`)
	devinFamily = regexp.MustCompile(`(?:^|[/.:])(?:swe-|devin)`)
)

// fusionLead returns the lead model of a Devin "fusion-<lead>-sidekick-<other>"
// ID. The lead does the work; the sidekick only assists, so it must not decide
// the author.
func fusionLead(id string) string {
	if rest, ok := strings.CutPrefix(id, "fusion-"); ok {
		lead, _, _ := strings.Cut(rest, "-sidekick-")
		return lead
	}
	return id
}

// authorEmail returns the commit email for a lowercased model ID, or "" when the
// family has no verified GitHub account.
func authorEmail(id string) string {
	id = fusionLead(id)
	switch {
	case strings.Contains(id, "claude"):
		return "noreply@anthropic.com"
	case strings.Contains(id, "gemini"):
		return "176961590+gemini-code-assist[bot]@users.noreply.github.com"
	case codexFamily.MatchString(id):
		return "267193182+codex@users.noreply.github.com"
	case devinFamily.MatchString(id):
		return "158243242+devin-ai-integration[bot]@users.noreply.github.com"
	}
	return ""
}

// cleanName drops the characters that would break a quoted identity and folds
// line breaks and runs of space into single spaces.
func cleanName(name string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if strings.ContainsRune("<>\"'", r) {
			return -1
		}
		return r
	}, name)), " ")
}

// authorFor returns the commit identity for the model, or ok=false when its
// family has no verified GitHub account. The name is the display name without
// catalog annotations such as "(latest)", falling back to the model ID.
func authorFor(model *ai.Model) (identity string, ok bool) {
	email := authorEmail(strings.ToLower(model.ID))
	if email == "" {
		return "", false
	}
	display, _, _ := strings.Cut(model.DisplayName, "(")
	name := cleanName(display)
	if name == "" {
		name = cleanName(model.ID)
	}
	return name + " <" + email + ">", true
}
