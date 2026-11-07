// Package yamljson turns a value yaml12.Parse or yaml12.ParseOrdered built into the JSON text JSON.stringify would write for
// it, so a YAML document can take the validation and decoding path of its JSON spelling. Objects keep the property order a
// JavaScript object enumerates: array-index keys first, ascending, then the others in insertion order.
//
// Ports the JSON.stringify step of packages/coding-agent/src/core/model-config.ts: its YAML branch parses a models.yml with
// the yaml package and validates the result against the same TypeBox schema as models.json, whose check reads the parsed
// value as JSON.
package yamljson

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/internal/jsnumber"
	"github.com/MichaelKinsy/PiG/internal/yaml12"
)

// Marshal returns the JSON text of value, a tree built by yaml12.Parse or yaml12.ParseOrdered. A value only an explicit
// YAML tag builds and JSON.stringify has no member for is an error: a symbol in an object's values, or a JavaScript Set,
// Map, Date or binary value. JSON.stringify writes `{}` for a Set or Map and the ISO string for a Date, which this follows.
func Marshal(value any) ([]byte, error) {
	var out strings.Builder
	if err := write(&out, value); err != nil {
		return nil, err
	}
	return []byte(out.String()), nil
}

func write(out *strings.Builder, value any) error {
	switch value := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		out.WriteString(strconv.FormatBool(value))
	case float64:
		out.Write(jsnumber.JSON(value))
	case string:
		writeString(out, value)
	case []any:
		out.WriteByte('[')
		for i, element := range value {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeArrayItem(out, element); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case *yaml12.Object:
		return writeObject(out, value.Keys, value.Values)
	case map[string]any:
		// yaml12.Parse builds a plain map, which has no insertion order to keep; its keys are written in JavaScript's
		// enumeration order so a caller that mixed both parsers still gets deterministic output.
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		return writeObject(out, keys, value)
	case yaml12.Date:
		writeString(out, isoDate(value.MS))
	case yaml12.Set, yaml12.Map:
		out.WriteString("{}")
	case yaml12.Symbol:
		out.WriteString("null")
	case []byte:
		out.WriteString("{}")
	default:
		return errors.New("models config holds a value JSON cannot represent (" + typeName(value) + ")")
	}
	return nil
}

// writeArrayItem writes an array element: a Symbol has no JSON form and becomes null, as JavaScript leaves it.
func writeArrayItem(out *strings.Builder, value any) error {
	if _, symbol := value.(yaml12.Symbol); symbol {
		out.WriteString("null")
		return nil
	}
	return write(out, value)
}

func writeObject(out *strings.Builder, keys []string, values map[string]any) error {
	ordered := javaScriptKeyOrder(keys)
	out.WriteByte('{')
	written := 0
	for _, key := range ordered {
		value, present := values[key]
		if !present {
			continue
		}
		if _, symbol := value.(yaml12.Symbol); symbol {
			// JSON.stringify omits a property whose value is undefined, which is what a symbol turns into.
			continue
		}
		if written > 0 {
			out.WriteByte(',')
		}
		written++
		writeString(out, key)
		out.WriteByte(':')
		if err := write(out, value); err != nil {
			return err
		}
	}
	out.WriteByte('}')
	return nil
}

// writeString writes s as a JSON string literal. Every character JSON allows stays as it is, including <, >, & and the line
// separators U+2028 and U+2029, as JavaScript writes them; the quote, the backslash and control characters are escaped.
func writeString(out *strings.Builder, s string) {
	out.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			if r < 0x20 {
				out.WriteString(`\u`)
				const hex = "0123456789abcdef"
				out.WriteByte(hex[r>>12&0xf])
				out.WriteByte(hex[r>>8&0xf])
				out.WriteByte(hex[r>>4&0xf])
				out.WriteByte(hex[r&0xf])
				continue
			}
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
}

// isoDate is Date.prototype.toISOString's text for the time value ms. A non-finite time is an invalid date, whose
// toISOString throws; the text then names it, and validation rejects the resulting value.
func isoDate(ms float64) string {
	if math.IsNaN(ms) || math.IsInf(ms, 0) {
		return "Invalid Date"
	}
	return time.UnixMilli(int64(ms)).UTC().Format("2006-01-02T15:04:05.000Z")
}

// javaScriptKeyOrder is the order Object.keys gives: the array-index keys ("0" up to 4294967294) ascending, then the other
// keys in insertion order.
func javaScriptKeyOrder(keys []string) []string {
	var indexes, others []string
	for _, key := range keys {
		if _, ok := arrayIndex(key); ok {
			indexes = append(indexes, key)
		} else {
			others = append(others, key)
		}
	}
	sort.SliceStable(indexes, func(i, j int) bool {
		a, _ := arrayIndex(indexes[i])
		b, _ := arrayIndex(indexes[j])
		return a < b
	})
	return append(indexes, others...)
}

// arrayIndex reports whether key is a canonical array index: decimal digits without a leading zero, below 2^32-1.
func arrayIndex(key string) (uint64, bool) {
	if key == "" || (len(key) > 1 && key[0] == '0') {
		return 0, false
	}
	value, err := strconv.ParseUint(key, 10, 64)
	if err != nil || value > math.MaxUint32-1 {
		return 0, false
	}
	return value, true
}

func typeName(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "unknown"
	}
	return string(data)
}
