package subagents

// Ports src/model-resolver.ts: stable scoring over available/configured models.
import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
)

var modelParts = regexp.MustCompile(`[\s\-/]+`)
var modelDate = regexp.MustCompile(`^\d{8}$`)

func ResolveModel(input string, models []*ai.Model) (*ai.Model, error) {
	normalize := func(s string) string { return strings.ReplaceAll(strings.ToLower(s), ".", "-") }
	query := normalize(input)
	var best *ai.Model
	bestScore := 0.0
	for _, m := range models {
		provider := m.ProviderMeta.ProviderID
		id, name, full := normalize(m.ID), normalize(m.DisplayName), normalize(provider+"/"+m.ID)
		score := 0.0
		switch {
		case id == query || full == query:
			score = 100
		case strings.Contains(id, query) || strings.Contains(full, query):
			score = 60 + float64(len(query))/float64(max(1, len(id)))*30
		case strings.Contains(name, query):
			score = 40 + float64(len(query))/float64(max(1, len(name)))*20
		default:
			all := true
			for _, part := range modelParts.Split(query, -1) {
				if !modelDate.MatchString(part) && !strings.Contains(id, part) && !strings.Contains(name, part) && !strings.Contains(strings.ToLower(provider), part) {
					all = false
					break
				}
			}
			if all {
				score = 20
			}
		}
		if score > bestScore {
			best, bestScore = m, score
		}
	}
	if best != nil && bestScore >= 20 {
		return best, nil
	}
	if _, bare, ok := strings.Cut(input, "/"); ok {
		if model, err := ResolveModel(bare, models); err == nil {
			return model, nil
		}
	}
	var names []string
	for _, m := range models {
		names = append(names, "  "+m.ProviderMeta.ProviderID+"/"+m.ID)
	}
	sort.Strings(names)
	return nil, fmt.Errorf("Model not found: %q.\n\nAvailable models:\n%s", input, strings.Join(names, "\n"))
}
