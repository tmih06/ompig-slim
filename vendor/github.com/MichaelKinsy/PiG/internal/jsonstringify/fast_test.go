package jsonstringify

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// BenchmarkCanonicalSession measures already-canonical retained tool output, from
// a short RPC frame to a long source-file result. Reused dst isolates scanning
// from buffer allocation; CPU and allocation profiles identify scan overhead.
func BenchmarkCanonicalSession(b *testing.B) {
	for _, size := range []struct {
		name string
		n    int
	}{{"frame", 128}, {"file", 8192}, {"history", 1 << 20}} {
		b.Run(size.name, func(b *testing.B) {
			text := strings.Repeat("def fixture(): return 42\n", size.n/24+1)
			raw, err := canonicalize([]byte(`{"role":"tool","content":` + string(mustJSON(b, text)) + `}`))
			if err != nil {
				b.Fatal(err)
			}
			dst := make([]byte, 0, len(raw))
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				out, err := AppendCanonical(dst[:0], raw)
				if err != nil || len(out) != len(raw) {
					b.Fatalf("canonicalization: %v", err)
				}
			}
		})
	}
}

// mustJSON encodes fixture strings independently of the scanner under test.
func mustJSON(tb testing.TB, value string) []byte {
	tb.Helper()
	out, err := json.Marshal(value)
	if err != nil {
		tb.Fatal(err)
	}
	return out
}

// TestCanonicalStrings protects token boundaries, escape handling, UTF-8 and
// surrogate pairing when the scanner skips long unescaped source-text runs.
func TestCanonicalStrings(t *testing.T) {
	for _, middle := range []string{
		`plain`, `\n`, `\"`, `\\`, `\u0000`, `\ud800`, `\udfff`,
		`\ud800\udfff`, `\u0061`, `\u000a`, `\/`, `é`, `😀`,
		"\x01", "\xff", `"`,
	} {
		for padding := 0; padding < 40; padding++ {
			raw := []byte(`{"text":"` + strings.Repeat("x", padding) + middle + strings.Repeat("y", 41-padding) + `"}`)
			if !json.Valid(raw) {
				if IsCanonical(raw) {
					t.Fatalf("invalid input accepted: %q", raw)
				}
				continue
			}
			want, err := canonicalize(raw)
			if err != nil {
				t.Fatal(err)
			}
			if IsCanonical(raw) && !bytes.Equal(raw, want) {
				t.Fatalf("false canonical proof: %q => %q", raw, want)
			}
			got, err := Canonicalize(raw)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("canonicalize %q = %q, %v; want %q", raw, got, err, want)
			}
		}
	}
}

// FuzzCanonicalProof compares the fast proof with the independent full rewrite.
// Only valid JSON is in Canonicalize's input contract; invalid UTF-8 is still
// tested because encoding/json accepts and replaces it during decoding.
func FuzzCanonicalProof(f *testing.F) {
	for _, seed := range []string{`{"text":"source\\noutput"}`, `{"a":1,"a":2}`, `["é","\ud800",0.000001]`, `{"0":1,"b":2}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		if !json.Valid(raw) {
			return
		}
		want, err := canonicalize(raw)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Canonicalize(raw)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("canonicalize %q = %q, %v; want %q", raw, got, err, want)
		}
	})
}
