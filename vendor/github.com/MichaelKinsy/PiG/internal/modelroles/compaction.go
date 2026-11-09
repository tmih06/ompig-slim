package modelroles

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// CompactionPoint is a scaled token base, exact fixed trigger, or window percentage.
// Zero fields mean no entry. Parsing never rounds a fractional token input.
type CompactionPoint struct {
	Tokens  int
	Percent float64
	Fixed   bool
}

var compactionInput = regexp.MustCompile(`(?i)^(f)?([0-9]+)([kmb])?$`)

// ParseCompactionInput accepts OMP's 90k, f400k, and 80% input; empty means reset.
func ParseCompactionInput(input string) (CompactionPoint, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return CompactionPoint{}, nil
	}
	if strings.HasSuffix(value, "%") {
		percent, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err == nil && percent > 0 && percent <= 100 {
			return CompactionPoint{Percent: percent}, nil
		}
		return CompactionPoint{}, fmt.Errorf("percentage must be in (0, 100]")
	}
	match := compactionInput.FindStringSubmatch(value)
	if match != nil {
		number, err := strconv.ParseFloat(match[2], 64)
		scale := 1.0
		switch strings.ToLower(match[3]) {
		case "k":
			scale = 1000
		case "m":
			scale = 1000000
		case "b":
			scale = 1000000000
		}
		number *= scale
		if err == nil && number > 0 && number <= 1<<53-1 {
			return CompactionPoint{Tokens: int(number), Fixed: match[1] != ""}, nil
		}
	}
	return CompactionPoint{}, fmt.Errorf("Invalid compaction point %q: use 90k, f400k, or 80%%", value)
}

// Raw emits OMP's persisted number/string forms, not UI abbreviations.
func (p CompactionPoint) Raw() json.RawMessage {
	var value any
	switch {
	case p.Fixed:
		value = fmt.Sprintf("f%d", p.Tokens)
	case p.Tokens > 0:
		value = p.Tokens
	case p.Percent > 0:
		value = strconv.FormatFloat(p.Percent, 'f', -1, 64) + "%"
	default:
		return nil
	}
	raw, _ := json.Marshal(value)
	return raw
}

// ParseCompactionEntry accepts only the persisted grammar: tokens must be integer
// numbers and fixed strings carry no k/m suffix. Null is an explicit tombstone.
func ParseCompactionEntry(raw json.RawMessage) (CompactionPoint, error) {
	if string(raw) == "null" || len(raw) == 0 {
		return CompactionPoint{}, nil
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		if number > 0 && math.Trunc(number) == number && number <= 1<<53-1 {
			return CompactionPoint{Tokens: int(number)}, nil
		}
		return CompactionPoint{}, fmt.Errorf("invalid token count")
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return CompactionPoint{}, fmt.Errorf("expected token count, fixed trigger, or percentage")
	}
	return ParseCompactionInput(value)
}

// CompactionTrigger applies fixed/percentage overrides, or scales the ordinary
// policy by a model-specific base. Fixed values at the window cap to its reserve.
func CompactionTrigger(window, reserve int, percent float64, fixed int, point CompactionPoint) int {
	if point.Fixed {
		fixed = point.Tokens
	} else if point.Percent > 0 {
		percent = point.Percent
		fixed = 0
	} else if point.Tokens > 0 {
		window = min(window, point.Tokens)
		fixed = 0
	}
	if fixed > 0 {
		if fixed >= window {
			return max(1, window-reserve)
		}
		return fixed
	}
	if percent > 0 {
		return max(1, int(float64(window)*math.Max(1, math.Min(99, percent))/100))
	}
	return max(1, window-reserve)
}
