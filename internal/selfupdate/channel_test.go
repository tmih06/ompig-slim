package selfupdate

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpdateChannels observes replacement, not the private release selector:
// Update's empty channel selects stable; crossing channels permits a downgrade.
// Handle's saved preference is exercised by the real-binary suite.
func TestUpdateChannels(t *testing.T) {
	for _, tc := range []struct {
		name, channel, current, target string
	}{
		{"default leaves beta", "", "2.0.0-beta.10", "1.0.0"},
		{"explicit stable leaves beta", "stable", "2.0.0-beta.10", "1.0.0"},
		{"beta leaves stable", "beta", "3.0.0", "2.0.0-beta.10"},
		{"beta increments numerically", "beta", "2.0.0-beta.9", "2.0.0-beta.10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := "#!/bin/sh\necho " + tc.target + "\n"
			archive := archiveFor(t, map[string]string{archiveMember: payload})
			requestedTag := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/releases/latest":
					fmt.Fprint(w, `{"tag_name":"v1.0.0"}`)
				case r.URL.Path == "/releases":
					if r.URL.Query().Get("page") == "2" {
						fmt.Fprint(w, `[{"tag_name":"v2.0.0-beta.10","prerelease":true}]`)
						return
					}
					w.Header().Set("Link", `</releases?page=2>; rel="next"`)
					fmt.Fprint(w, `[{"tag_name":"v2.0.0-beta.9","prerelease":true},
						{"tag_name":"v9.0.0-beta.99","prerelease":true,"draft":true},
						{"tag_name":"v9.0.0-rc.1","prerelease":true},
						{"tag_name":"v9.0.0"}]`)
				case strings.HasSuffix(r.URL.Path, ".sha256"):
					fmt.Fprintln(w, digestOf(archive)+"  archive.tar.gz")
				default:
					requestedTag <- strings.Split(r.URL.Path, "/")[3]
					_, _ = w.Write(archive)
				}
			}))
			defer server.Close()
			exe := filepath.Join(t.TempDir(), "ompig-slim")
			if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			code := Update(Options{Version: tc.current, Channel: tc.channel, ExePath: exe,
				DownloadBase: server.URL + "/releases", ReleasesAPI: server.URL + "/releases/latest",
				Client: server.Client(), Stderr: &stderr})
			if code != 0 {
				t.Fatalf("Update = %d: %s", code, &stderr)
			}
			got, err := os.ReadFile(exe)
			var tag string
			select {
			case tag = <-requestedTag:
			default:
			}
			if err != nil || string(got) != payload || tag != "v"+tc.target {
				t.Fatalf("replacement = %q, tag = %q, error = %v", got, tag, err)
			}
		})
	}
}

// TestChannelRefusalsLeaveBinaryUntouched covers an older beta, missing beta,
// invalid channel response, and a stable endpoint accidentally serving beta.
func TestChannelRefusalsLeaveBinaryUntouched(t *testing.T) {
	for _, tc := range []struct {
		name, channel, body string
		status, wantCode    int
	}{
		{"older beta", "beta", `[{"tag_name":"v2.0.0-beta.9","prerelease":true}]`, 200, 0},
		{"no beta", "beta", `[]`, 200, 1},
		{"bad JSON", "beta", `{`, 200, 1},
		{"rate limit", "beta", `{}`, 403, 1},
		{"stable is not beta", "stable", `{"tag_name":"v3.0.0-beta.1","prerelease":true}`, 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			exe := filepath.Join(t.TempDir(), "ompig-slim")
			if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			code := Update(Options{Version: "2.0.0-beta.10", Channel: tc.channel,
				ExePath: exe, ReleasesAPI: server.URL + "/latest", Stderr: &stderr})
			if code != tc.wantCode {
				t.Fatalf("Update = %d, want %d: %s", code, tc.wantCode, &stderr)
			}
			got, err := os.ReadFile(exe)
			if err != nil || string(got) != "old" {
				t.Fatalf("binary changed to %q: %v", got, err)
			}
		})
	}
}

// TestHandleInvalidChannel ensures typos cannot silently select stable or
// delegate a self-update to PiG's unrelated package parser.
func TestHandleInvalidChannel(t *testing.T) {
	for _, args := range [][]string{
		{"update", "--channel"}, {"upgrade", "--channel=nightly"}, {"update", "--channel="},
	} {
		var stderr bytes.Buffer
		_, handled, code := Handle(args, "1.0.0", &bytes.Buffer{}, &stderr)
		if !handled || code != 2 || !strings.Contains(stderr.String(), "stable or beta") {
			t.Fatalf("Handle(%q) = handled %v, code %d, stderr %q", args, handled, code, &stderr)
		}
	}
}
