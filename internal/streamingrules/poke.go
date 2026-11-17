package streamingrules

import (
	"encoding/json"
	"strings"

	"github.com/MichaelKinsy/PiG/coding/extension"
	"github.com/MichaelKinsy/PiG/tui"
)

const pokeType = "streaming-rule-poke"

type pokeRule struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

type poke struct {
	Blocked bool       `json:"blocked"`
	Rules   []pokeRule `json:"rules"`
}

func pokeDraft(rules []*rule, blocked bool) extension.SessionBoundaryDraft {
	data := poke{Blocked: blocked}
	for _, rule := range rules {
		data.Rules = append(data.Rules, pokeRule{Name: rule.name, Summary: rule.summary})
	}
	return extension.SessionBoundaryDraft{
		Type: extension.SessionBoundaryDraftCustom, CustomType: pokeType, Data: data,
	}
}

func renderPoke(entry extension.CustomEntry, _ extension.EntryRenderOptions, _ extension.Theme) extension.Component {
	raw, err := json.Marshal(entry.Data)
	if err != nil {
		return nil
	}
	var data poke
	if err := json.Unmarshal(raw, &data); err != nil || len(data.Rules) == 0 {
		return nil
	}
	theme := tui.ActiveTheme()
	outcome := "Reminder sent"
	if data.Blocked {
		outcome = "Tool call blocked"
	}
	box := tui.NewPaddedBox(1, 0, func(text string) string { return theme.Bg("customMessageBg", text) })
	box.AddChild(tui.NewText(theme.Fg("accent", "◇ Rule poke") + theme.Fg("muted", " · "+outcome)))
	for _, rule := range data.Rules {
		box.AddChild(tui.NewText(theme.Fg("customMessageLabel", rule.Name) + " · " + theme.Fg("customMessageText", rule.Summary)))
	}
	return box
}

func summarizePattern(description, body string) string {
	if strings.TrimSpace(description) == "" {
		description, _, _ = strings.Cut(body, "\n\n")
	}
	summary := []rune(strings.Join(strings.Fields(description), " "))
	if len(summary) > 160 {
		return string(summary[:157]) + "..."
	}
	return string(summary)
}
