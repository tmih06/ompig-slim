package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// archiveFor builds a gzipped tar in the release layout with the given members.
func archiveFor(t *testing.T, members map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	packageWriter := tar.NewWriter(gz)
	for name, content := range members {
		header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if err := packageWriter.WriteHeader(header); err != nil {
			t.Fatalf("write header %s: %v", name, err)
		}
		if _, err := packageWriter.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := packageWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buffer.Bytes()
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		name   string
		left   string
		right  string
		want   int
		wantOK bool
	}{
		{"equal", "1.2.3", "1.2.3", 0, true},
		{"major", "2.0.0", "1.9.9", 1, true},
		{"minor", "1.2.0", "1.10.0", -1, true},
		{"patch", "1.2.4", "1.2.3", 1, true},
		{"release beats prerelease", "1.0.0", "1.0.0-rc.1", 1, true},
		{"prerelease loses to release", "1.0.0-rc.1", "1.0.0", -1, true},
		{"prerelease order", "1.0.0-beta", "1.0.0-alpha", 1, true},
		{"build metadata ignored", "1.0.0+abc", "1.0.0+def", 0, true},
		{"leading zeros are numbers", "1.02.0", "1.2.0", 0, true},
		{"not a version", "nightly", "1.0.0", 0, false},
		{"two components", "1.0", "1.0.0", 0, false},
		{"four components", "1.0.0.0", "1.0.0", 0, false},
		{"negative", "1.-1.0", "1.0.0", 0, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := compareVersions(testCase.left, testCase.right)
			if ok != testCase.wantOK {
				t.Fatalf("compareVersions(%q, %q) ok = %v, want %v", testCase.left, testCase.right, ok, testCase.wantOK)
			}
			if ok && got != testCase.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", testCase.left, testCase.right, got, testCase.want)
			}
		})
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		candidate string
		current   string
		want      bool
	}{
		{"2.0.0", "1.0.0", true},
		{"1.0.0", "2.0.0", false},
		{"1.0.0", "1.0.0", false},
		{"1.0.0-rc.1", "1.0.0", false},
		// An unparsable pair still offers the update rather than reporting a
		// silent success, so a maintainer notices the odd tag.
		{"nightly", "stable", true},
		{"same", "same", false},
	}
	for _, testCase := range cases {
		if got := isNewer(testCase.candidate, testCase.current); got != testCase.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", testCase.candidate, testCase.current, got, testCase.want)
		}
	}
}

func TestArchiveNameFor(t *testing.T) {
	cases := []struct {
		goos, goarch string
		want         string
		err          bool
	}{
		{"linux", "amd64", "ompig-slim-linux-x64.tar.gz", false},
		{"linux", "arm64", "ompig-slim-linux-arm64.tar.gz", false},
		{"darwin", "amd64", "ompig-slim-darwin-x64.tar.gz", false},
		{"darwin", "arm64", "ompig-slim-darwin-arm64.tar.gz", false},
		{"windows", "amd64", "", true},
		{"linux", "riscv64", "", true},
		{"linux", "386", "", true},
	}
	for _, testCase := range cases {
		got, err := archiveNameFor(testCase.goos, testCase.goarch)
		if testCase.err {
			if err == nil {
				t.Errorf("archiveNameFor(%s, %s) = %q, want an error", testCase.goos, testCase.goarch, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("archiveNameFor(%s, %s): %v", testCase.goos, testCase.goarch, err)
			continue
		}
		if got != testCase.want {
			t.Errorf("archiveNameFor(%s, %s) = %q, want %q", testCase.goos, testCase.goarch, got, testCase.want)
		}
	}
}

// releaseServer serves one tag's archive and checksum, plus the releases API.
func releaseServer(t *testing.T, tag string, archive []byte, checksum string) *httptest.Server {
	t.Helper()
	name, err := archiveNameFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip("no release archive for this platform")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": tag})
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = io.WriteString(w, checksum+"  "+name+"\n")
		case strings.HasSuffix(r.URL.Path, name):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestUpdateReplacesTheExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the release archives cover Linux and macOS only")
	}
	archive := archiveFor(t, map[string]string{archiveMember: "#!/bin/sh\necho 2.0.0\n"})
	server := releaseServer(t, "v2.0.0", archive, digestOf(archive))

	dir := t.TempDir()
	executable := filepath.Join(dir, "ompig-slim")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\necho 1.0.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Update(Options{
		Version:      "v1.0.0",
		ExePath:      executable,
		DownloadBase: server.URL + "/releases",
		ReleasesAPI:  server.URL + "/releases/latest",
		Client:       server.Client(),
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if code != 0 {
		t.Fatalf("Update = %d, want 0; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	replaced, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(replaced), "2.0.0") {
		t.Fatalf("executable was not replaced: %q", replaced)
	}
	info, err := os.Stat(executable)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("replaced executable is not executable: %v", info.Mode())
	}
	// The summary names both releases on one settled line.
	if !strings.Contains(stdout.String(), "Updated ompig-slim 1.0.0 → 2.0.0") {
		t.Fatalf("stdout did not report the update: %q", stdout.String())
	}
	// The mark is printed once per run, before the progress.
	if !strings.Contains(stdout.String(), "in Go \u00b7 slim harness") {
		t.Fatalf("stdout did not print the banner: %q", stdout.String())
	}
	// No staging file may survive beside the executable, which is on PATH.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("left %d entries in the install directory, want 1", len(entries))
	}
}

func TestUpdateIsANoOpWhenCurrent(t *testing.T) {
	archive := archiveFor(t, map[string]string{archiveMember: "new"})
	server := releaseServer(t, "v1.0.0", archive, digestOf(archive))
	dir := t.TempDir()
	executable := filepath.Join(dir, "ompig-slim")
	before := []byte("current")
	if err := os.WriteFile(executable, before, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Update(Options{
		Version: "1.0.0", ExePath: executable,
		DownloadBase: server.URL + "/releases", ReleasesAPI: server.URL + "/releases/latest",
		Client: server.Client(), Stdout: &stdout, Stderr: &stderr,
	})
	if code != 0 {
		t.Fatalf("Update = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "up to date") {
		t.Fatalf("stdout = %q, want an up-to-date line", stdout.String())
	}
	// No banner for a run that changes nothing: it is not an update.
	if strings.Contains(stdout.String(), "in Go") {
		t.Fatalf("an up-to-date run printed the banner: %q", stdout.String())
	}
	after, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("executable changed to %q", after)
	}
}

func TestUpdateRefusesABadChecksum(t *testing.T) {
	archive := archiveFor(t, map[string]string{archiveMember: "new"})
	server := releaseServer(t, "v2.0.0", archive, strings.Repeat("0", 64))
	dir := t.TempDir()
	executable := filepath.Join(dir, "ompig-slim")
	before := []byte("current")
	if err := os.WriteFile(executable, before, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Update(Options{
		Version: "1.0.0", ExePath: executable,
		DownloadBase: server.URL + "/releases", ReleasesAPI: server.URL + "/releases/latest",
		Client: server.Client(), Stdout: &stdout, Stderr: &stderr,
	})
	if code == 0 {
		t.Fatalf("Update = 0, want a failure for a bad checksum")
	}
	if !strings.Contains(stderr.String(), "checksum mismatch") {
		t.Fatalf("stderr = %q, want a checksum mismatch", stderr.String())
	}
	after, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("executable changed despite a bad checksum: %q", after)
	}
}

func TestUpdateRefusesAnArchiveWithoutTheExecutable(t *testing.T) {
	archive := archiveFor(t, map[string]string{"ompig-slim/README.md": "doc"})
	server := releaseServer(t, "v2.0.0", archive, digestOf(archive))
	dir := t.TempDir()
	executable := filepath.Join(dir, "ompig-slim")
	before := []byte("current")
	if err := os.WriteFile(executable, before, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Update(Options{
		Version: "1.0.0", ExePath: executable,
		DownloadBase: server.URL + "/releases", ReleasesAPI: server.URL + "/releases/latest",
		Client: server.Client(), Stdout: &stdout, Stderr: &stderr,
	})
	if code == 0 {
		t.Fatalf("Update = 0, want a failure for a release without the executable")
	}
	if !strings.Contains(stderr.String(), "does not contain") {
		t.Fatalf("stderr = %q, want a missing-member error", stderr.String())
	}
	after, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("executable changed despite a bad archive: %q", after)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("left %d entries in the install directory, want 1", len(entries))
	}
}

func TestUpdateReportsAnUnreachableChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := Update(Options{
		Version: "1.0.0", ExePath: filepath.Join(t.TempDir(), "ompig-slim"),
		DownloadBase: server.URL + "/releases", ReleasesAPI: server.URL + "/releases/latest",
		Client: server.Client(), Stdout: &stdout, Stderr: &stderr,
	})
	if code == 0 {
		t.Fatalf("Update = 0, want a failure when the channel is unreachable")
	}
	if !strings.Contains(stderr.String(), "release channel") {
		t.Fatalf("stderr = %q, want a release-channel error", stderr.String())
	}
	if !strings.Contains(stderr.String(), "install.sh") {
		t.Fatalf("stderr = %q, want the installer fallback", stderr.String())
	}
}

// withMarker points the config root at a temporary directory and writes the
// marker scripts/install.sh would create.
func withMarker(t *testing.T, downloadBase string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("PIG_HOME", root)
	content := "kind=archive\ndownload-base=" + downloadBase + "\n"
	if err := os.WriteFile(filepath.Join(root, markerName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveRequest(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		want  request
		claim bool
	}{
		{"bare update", []string{"update"}, request{self: true}, true},
		{"bare upgrade", []string{"upgrade"}, request{self: true}, true},
		{"self flag", []string{"update", "--self"}, request{self: true}, true},
		{"self name", []string{"update", "self"}, request{self: true}, true},
		{"app name", []string{"update", "ompig-slim"}, request{self: true}, true},
		{"pig name", []string{"update", "pig"}, request{self: true}, true},
		{"force", []string{"update", "--force"}, request{self: true, force: true}, true},
		{"all", []string{"update", "--all"}, request{self: true, all: true}, true},
		{"trust flag alone still updates", []string{"update", "--approve"},
			request{self: true}, true},
		// PiG resolves an explicit self target with --extensions to --all.
		{"self with extensions", []string{"update", "self", "--extensions"},
			request{self: true, all: true}, true},
		{"self flag with extensions", []string{"update", "--self", "--extensions"},
			request{self: true, all: true}, true},
		{"help", []string{"update", "--help"}, request{}, false},
		{"short help", []string{"update", "-h"}, request{}, false},
		{"models", []string{"update", "--models"}, request{}, false},
		{"packages only", []string{"update", "--extensions"}, request{}, false},
		{"one package", []string{"update", "--extension", "x"}, request{}, false},
		{"positional source", []string{"update", "npm:@example/pkg"}, request{}, false},
		{"unknown flag", []string{"update", "--nonsense"}, request{}, false},
		{"unrelated command", []string{"list"}, request{}, false},
		{"no arguments", nil, request{}, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, claimed := resolveRequest(testCase.args)
			if claimed != testCase.claim {
				t.Fatalf("resolveRequest(%q) claimed = %v, want %v", testCase.args, claimed, testCase.claim)
			}
			if got != testCase.want {
				t.Fatalf("resolveRequest(%q) = %+v, want %+v", testCase.args, got, testCase.want)
			}
		})
	}
}

func TestCanonicalUpdateRewritesOnlyTheAlias(t *testing.T) {
	original := []string{"upgrade", "--models"}
	got := canonicalUpdate(original)
	if got[0] != "update" || got[1] != "--models" {
		t.Fatalf("canonicalUpdate = %q", got)
	}
	// The caller's slice must not be modified in place.
	if original[0] != "upgrade" {
		t.Fatalf("canonicalUpdate modified its input: %q", original)
	}
	// A command that is not the alias passes through unchanged.
	plain := []string{"update", "--models"}
	passed := canonicalUpdate(plain)
	if len(passed) != 2 || passed[0] != "update" || passed[1] != "--models" {
		t.Fatalf("canonicalUpdate(%q) = %q", plain, passed)
	}
}

func TestHandleDeclinesWithoutTheMarker(t *testing.T) {
	t.Setenv("PIG_HOME", t.TempDir())
	for _, args := range [][]string{{"update"}, {"upgrade"}, {"update", "--force"}} {
		delegated, handled, _ := Handle(args, "1.0.0", io.Discard, io.Discard)
		if handled {
			t.Fatalf("Handle(%q) claimed a binary without the archive marker", args)
		}
		if delegated[0] != "update" {
			t.Fatalf("Handle(%q) delegated %q, want the canonical update", args, delegated)
		}
	}
}

func TestReadMarker(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PIG_HOME", root)
	marker := filepath.Join(root, markerName)

	if _, err := readMarker(); err == nil {
		t.Fatal("readMarker accepted a missing marker")
	}

	if err := os.WriteFile(marker, []byte("kind=npm\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readMarker(); err == nil {
		t.Fatal("readMarker accepted a foreign kind")
	}

	content := "kind=archive\nexecutable=/usr/local/bin/ompig-slim\ndownload-base=https://example.test/releases\n"
	if err := os.WriteFile(marker, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readMarker()
	if err != nil {
		t.Fatalf("readMarker: %v", err)
	}
	if got.DownloadBase != "https://example.test/releases" {
		t.Fatalf("DownloadBase = %q", got.DownloadBase)
	}
	if got.ExecutablePath != "/usr/local/bin/ompig-slim" {
		t.Fatalf("ExecutablePath = %q", got.ExecutablePath)
	}
}

func TestSameExecutable(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "ompig-slim")
	if err := os.WriteFile(installed, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "on-path")
	if err := os.Symlink(installed, link); err != nil {
		t.Fatal(err)
	}
	if !sameExecutable(installed, installed) {
		t.Error("sameExecutable rejected the same path")
	}
	if !sameExecutable(link, installed) {
		t.Error("sameExecutable did not follow a symlink to the installed file")
	}
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if sameExecutable(other, installed) {
		t.Error("sameExecutable accepted a different executable")
	}
	if sameExecutable("", installed) || sameExecutable(installed, "") {
		t.Error("sameExecutable accepted an empty path")
	}
}
