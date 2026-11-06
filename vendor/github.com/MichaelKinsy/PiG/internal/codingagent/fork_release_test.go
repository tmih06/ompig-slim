package codingagent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type forkReleaseTransport func(*http.Request) (*http.Response, error)

func (f forkReleaseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestForkReleaseNotificationBoundaries(t *testing.T) {
	t.Setenv("PIG_HOME", t.TempDir())
	t.Setenv("PI_OFFLINE", "")
	t.Setenv("PIG_OFFLINE", "")
	t.Setenv("PI_SKIP_VERSION_CHECK", "")
	for _, tc := range []struct {
		name string
		status int
		body string
		wantVersion string
		wantNote string
		wantURL string
	}{
		{"newer", 200, `{"tag_name":"v0.2.0","body":"  Changes  ","html_url":"https://github.com/tmih06/omp-slim/releases/tag/v0.2.0"}`, "0.2.0", "Changes", "https://github.com/tmih06/omp-slim/releases/tag/v0.2.0"},
		{"same", 200, `{"tag_name":"v0.1.0"}`, "", "", ""},
		{"build metadata not newer", 200, `{"tag_name":"v0.1.0+build.2"}`, "", "", ""},
		{"older", 200, `{"tag_name":"v0.0.9"}`, "", "", ""},
		{"invalid version", 200, `{"tag_name":"nightly"}`, "", "", ""},
		{"no releases", 404, `{"message":"Not Found"}`, "", "", ""},
		{"invalid optional fields ignored", 200, `{"tag_name":"v0.2.0","body":3,"html_url":{}}`, "0.2.0", "", forkReleasePage},
		{"malformed metadata", 200, `{`, "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: forkReleaseTransport(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != forkReleaseURL {
					t.Fatalf("release probe left the personal fork: %s", request.URL)
				}
				return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body)), Request: request}, nil
			})}
			got := CheckForForkRelease(context.Background(), client, "0.1.0")
			if tc.wantVersion == "" {
				if got != nil { t.Fatalf("unexpected update: %+v", got) }
				return
			}
			if got == nil || got.Version != tc.wantVersion || got.HTMLURL != tc.wantURL {
				t.Fatalf("update = %+v, want version %q and URL %q", got, tc.wantVersion, tc.wantURL)
			}
			if (got.Note == nil && tc.wantNote != "") || (got.Note != nil && *got.Note != tc.wantNote) {
				t.Fatalf("note = %v, want %q", got.Note, tc.wantNote)
			}
			if got.Binary.URL != "" || got.Binary.SHA256 != "" {
				t.Fatal("unsigned release metadata must not authorize executable replacement")
			}
		})
	}
}

func TestForkReleaseOfflineSkipsNetwork(t *testing.T) {
	t.Setenv("PI_OFFLINE", "1")
	client := &http.Client{Transport: forkReleaseTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("offline startup must not make a release request")
		return nil, nil
	})}
	if got := CheckForForkRelease(context.Background(), client, "0.1.0"); got != nil {
		t.Fatalf("offline update = %+v", got)
	}
}
