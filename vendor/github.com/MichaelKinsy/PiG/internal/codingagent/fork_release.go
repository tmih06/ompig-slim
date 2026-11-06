package codingagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/internal/managementhttp"
	semver "github.com/MichaelKinsy/PiG/third_party/semver"
)

// The release channel remains the original personal fork; renaming the local
// executable does not rename or create a remote repository.
const forkReleaseURL = "https://api.github.com/repos/tmih06/omp-slim/releases/latest"
const forkReleasePage = "https://github.com/tmih06/omp-slim/releases/latest"

// CheckForForkRelease reports a newer published personal-fork release. It is a
// notification only: unsigned GitHub metadata can never replace the executable.
func CheckForForkRelease(ctx context.Context, client *http.Client, currentVersion string) *BinaryUpdate {
	if updateChecksOffline() || os.Getenv("PI_SKIP_VERSION_CHECK") != "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, forkReleaseURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", ai.FormatUserAgent(nil))
	req.Header.Set("Accept", "application/vnd.github+json")
	retries := 0
	resp, err := doUpdateRequest(client, req, managementhttp.FetchRetryOptions{MaxRetries: &retries, Timeout: 10 * time.Second})
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var document struct {
		TagName string          `json:"tag_name"`
		Body    json.RawMessage `json:"body"`
		HTMLURL json.RawMessage `json:"html_url"`
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, &document) != nil {
		return nil
	}
	version := strings.TrimSpace(document.TagName)
	if strings.HasPrefix(version, "v") && len(version) > 1 && version[1] >= '0' && version[1] <= '9' {
		version = version[1:]
	}
	if _, err := semver.StrictNewVersion(version); err != nil || !IsNewerPackageVersion(version, currentVersion) {
		return nil
	}
	var note, releaseURL string
	_ = json.Unmarshal(document.Body, &note)
	_ = json.Unmarshal(document.HTMLURL, &releaseURL)
	release := LatestPiRelease{Version: version, HTMLURL: strings.TrimSpace(releaseURL)}
	if release.HTMLURL == "" {
		release.HTMLURL = forkReleasePage
	}
	if note := strings.TrimSpace(note); note != "" {
		release.Note = &note
	}
	return &BinaryUpdate{LatestPiRelease: release, CurrentVersion: currentVersion}
}
