package jv

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// TestPlainEndForms compares both public scans with a scalar oracle at every
// offset. Arbitrary bytes and each stop lane protect the shared SWAR boundary
// rules without assuming UTF-8 validity or requiring aligned input.
func TestPlainEndForms(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 27))
	inputs := []string{"", strings.Repeat("x", 128)}
	for _, stop := range []byte{0, 0x1f, '"', '\\', 0x80, 0xff} {
		for lane := 0; lane < 40; lane++ {
			inputs = append(inputs, strings.Repeat("x", lane)+string(stop)+strings.Repeat("y", 64))
		}
	}
	for range 100 {
		data := make([]byte, rng.IntN(160))
		for i := range data {
			data[i] = byte(rng.IntN(256))
		}
		inputs = append(inputs, string(data))
	}
	for _, input := range inputs {
		data := []byte(input)
		for start := 0; start <= len(input); start++ {
			want := start
			for want < len(input) {
				c := input[want]
				if c < 0x20 || c == '"' || c == '\\' || c >= 0x80 {
					break
				}
				want++
			}
			if got := PlainEnd(input, start); got != want {
				t.Fatalf("string scan %q at %d = %d; want %d", input, start, got, want)
			}
			if got := PlainEndBytes(data, start); got != want {
				t.Fatalf("byte scan %q at %d = %d; want %d", input, start, got, want)
			}
		}
	}
}
