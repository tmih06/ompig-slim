// Package selfupdate downloads verified releases for archive installations.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	semver "github.com/MichaelKinsy/PiG/third_party/semver"

	"ompig-slim/internal/configroot"
)

const (
	// stageReplacementArg asks the freshly downloaded binary to materialize its
	// embedded extension SDKs before it replaces the running executable. Running
	// the new binary is necessary because this process still has the old SDK.
	StageReplacementArg = "--internal-stage-extension-sdks"

	// updateTimeout bounds release lookup, downloads, and SDK staging.
	updateTimeout = 5 * time.Minute

	// maxArchiveBytes bounds both the download and the extracted executable.
	// It matches PiG's own self-update limit so neither path accepts more.
	maxArchiveBytes = 512 << 20

	// maxChecksumBytes is independent of the much larger release archive limit.
	maxChecksumBytes = 64 << 10

	defaultDownloadBase = "https://github.com/tmih06/ompig-slim/releases"
	defaultReleasesAPI  = "https://api.github.com/repos/tmih06/ompig-slim/releases/latest"

	markerName = "ompig-slim-install"

	// markerKind values distinguish an archive install from anything else.
	markerKind = "archive"
)

// Options configures an update without requiring the real executable or release host.
type Options struct {
	// Version is the running release, such as "0.1.0"; a leading "v" is
	// ignored. It is compared against the release tag.
	Version string
	// ExePath is the file to replace, normally the running executable.
	ExePath string
	// DownloadBase is the release root that serves
	// download/<tag>/<archive>, mirroring GitHub's layout.
	DownloadBase string
	// ReleasesAPI returns the latest stable release document. Beta selection
	// lists releases at the same URL with its trailing /latest removed.
	ReleasesAPI string
	// Channel selects stable (also the empty default) or beta. Handle resolves
	// and persists the installation's preference; Update only replaces the file.
	Channel string
	// Client performs every request. A nil client uses a fresh one bounded by
	// updateTimeout.
	Client *http.Client
	// Stdout receives progress and success lines; Stderr receives failures.
	Stdout io.Writer
	Stderr io.Writer
	// Force reinstalls the latest release even when the running version is
	// already current.
	Force bool
}

// Update downloads the latest verified release and replaces Options.ExePath.
// It returns 0 when current or replaced, 1 on failure, or 2 for an invalid channel.
func Update(o Options) int {
	return updateWithTimeout(o, updateTimeout)
}

func updateWithTimeout(o Options, timeout time.Duration) int {
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	if strings.TrimSpace(o.ExePath) == "" {
		fmt.Fprintln(o.Stderr, "Update failed: the running executable could not be located")
		return 1
	}
	if o.Channel != "" && o.Channel != "stable" && o.Channel != "beta" {
		fmt.Fprintln(o.Stderr, "Update failed: --channel must be stable or beta")
		return 2
	}
	unlock, err := lockUpdate(o.ExePath)
	if err != nil {
		fmt.Fprintf(o.Stderr, "Update failed: %v\n", err)
		return 1
	}
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return updateLocked(ctx, o, nil)
}

// updateLocked expects the caller to hold the update lock for o.ExePath and to
// supply non-nil Stdout and Stderr. The channel is checked again because Handle
// may take it from the install marker.
func updateLocked(ctx context.Context, o Options, savePreference func() error) int {
	if o.Client == nil {
		o.Client = &http.Client{Timeout: updateTimeout}
	}
	if o.Channel != "" && o.Channel != "stable" && o.Channel != "beta" {
		fmt.Fprintln(o.Stderr, "Update failed: --channel must be stable or beta")
		return 2
	}
	artifact, err := artifactFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		fmt.Fprintf(o.Stderr, "Update failed: %v\n", err)
		return 1
	}

	finish := func() int {
		if savePreference != nil {
			if err := savePreference(); err != nil {
				fmt.Fprintf(o.Stderr, "Update succeeded, but could not save the channel: %v\n", err)
				return 1
			}
		}
		return 0
	}
	current := strings.TrimPrefix(strings.TrimSpace(o.Version), "v")

	tag, err := o.latestTag(ctx)
	if err != nil {
		fmt.Fprintf(o.Stderr, "Update failed: could not reach the release channel: %v\n%s\n", err, fallback())
		return 1
	}
	latest := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	// Channel switches permit downgrades; same-channel updates require a newer version.
	switching := (o.Channel == "beta") != isBetaVersion(current)
	if !o.Force && !switching && !isNewer(latest, current) {
		fmt.Fprintf(o.Stdout, "ompig-slim %s is up to date.\n", current)
		return finish()
	}

	archiveURL := strings.TrimRight(o.DownloadBase, "/") + "/download/" + tag + "/" + artifact.name
	printBanner(o.Stdout)
	fmt.Fprintf(o.Stdout, "Updating ompig-slim %s → %s\n\n", current, latest)
	archive, err := o.download(ctx, archiveURL, o.Stderr)
	if err != nil {
		fmt.Fprintf(o.Stderr, "Update failed: %v\n%s\n", err, fallback())
		return 1
	}
	defer func() { _ = os.Remove(archive) }()

	if err := o.replace(ctx, archive, artifact); err != nil {
		fmt.Fprintf(o.Stderr, "Update failed: %v\n%s\n", err, fallback())
		return 1
	}

	fmt.Fprintf(o.Stdout, "\n✓ Updated ompig-slim %s → %s\n", current, latest)
	fmt.Fprintln(o.Stdout, "Extension SDKs prepared; startup can reuse source-extension caches.")
	return finish()
}

func fallback() string {
	const root = "https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/"
	if runtime.GOOS == "windows" {
		return "Reinstall with the PowerShell installer: " + root + "install.ps1"
	}
	return "Reinstall with the installer instead: curl -fsSL " + root + "install.sh | bash"
}

// latestTag returns the newest published release in the requested channel.
func (o Options) latestTag(ctx context.Context) (string, error) {
	if o.Channel == "beta" {
		return o.latestBetaTag(ctx)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.ReleasesAPI, nil)
	if err != nil {
		return "", err
	}
	// GitHub's API serves JSON to this Accept value and rejects requests with
	// no User-Agent.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ompig-slim-selfupdate")
	resp, err := o.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("release channel answered %s", resp.Status)
	}
	var document struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	// Bound the read: a release document is small, and this endpoint is remote.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&document); err != nil {
		return "", fmt.Errorf("parse release channel response: %w", err)
	}
	tag := strings.TrimSpace(document.TagName)
	version, parseErr := semver.NewVersion(strings.TrimPrefix(tag, "v"))
	if parseErr != nil || document.Draft || document.Prerelease || version.Prerelease() != "" {
		return "", errors.New("release channel did not name a stable release")
	}
	return tag, nil
}

// isBetaVersion recognizes the numbered beta line without treating RCs or
// malformed release tags as beta builds.
func isBetaVersion(tag string) bool {
	version, err := semver.NewVersion(strings.TrimPrefix(tag, "v"))
	return err == nil && strings.HasPrefix(version.Prerelease(), "beta.")
}

// latestBetaTag scans all release pages, ignoring drafts and other prerelease
// lines. Numeric semver ordering handles beta.10 versus beta.9, independently
// of publication order. Pagination stays on the configured host, not Link URLs.
func (o Options) latestBetaTag(ctx context.Context) (string, error) {
	endpoint, err := url.Parse(o.ReleasesAPI)
	if err != nil {
		return "", err
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/latest")
	best := ""
	for page := 1; ; page++ {
		query := endpoint.Query()
		query.Set("per_page", "100")
		query.Set("page", fmt.Sprint(page))
		endpoint.RawQuery = query.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "ompig-slim-selfupdate")
		resp, err := o.Client.Do(req)
		if err != nil {
			return "", err
		}
		var releases []struct {
			TagName    string `json:"tag_name"`
			Draft      bool   `json:"draft"`
			Prerelease bool   `json:"prerelease"`
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return "", fmt.Errorf("release channel answered %s", resp.Status)
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&releases)
		_ = resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("parse beta release channel: %w", err)
		}
		for _, release := range releases {
			version := "v" + strings.TrimPrefix(release.TagName, "v")
			if release.Draft || !release.Prerelease || !isBetaVersion(version) {
				continue
			}
			if best == "" || isNewer(strings.TrimPrefix(version, "v"), strings.TrimPrefix(best, "v")) {
				best = release.TagName
			}
		}
		if !strings.Contains(resp.Header.Get("Link"), `rel="next"`) {
			break
		}
	}
	if best == "" {
		return "", errors.New("release channel did not name a beta release")
	}
	return best, nil
}

// download fetches the archive and its published SHA-256 into the system
// temporary directory, verifying the digest before returning a path to the
// verified archive. The caller owns that file.
//
// The checksum comes from the same release as the archive, exactly as in
// scripts/install.sh: it detects a corrupt, truncated, or swapped download. It
// is not a signature and does not defend against a compromised release.
func (o Options) download(ctx context.Context, archiveURL string, progressOut io.Writer) (string, error) {
	meter := newProgress(progressOut, "download")
	archive, err := o.fetch(ctx, archiveURL, meter.set, maxArchiveBytes)
	if err != nil {
		return "", err
	}
	meter.finish("archive downloaded")
	// The checksum is small, so fetch it without progress reporting.
	checksumFile, err := o.fetch(ctx, archiveURL+".sha256", nil, maxChecksumBytes)
	if err != nil {
		_ = os.Remove(archive)
		return "", err
	}
	raw, err := os.ReadFile(checksumFile)
	_ = os.Remove(checksumFile)
	if err != nil {
		_ = os.Remove(archive)
		return "", fmt.Errorf("read checksum for %s: %w", archiveURL, err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		_ = os.Remove(archive)
		return "", fmt.Errorf("published checksum for %s is not a SHA-256 digest", filepath.Base(archiveURL))
	}
	expected := fields[0]
	if _, err := hex.DecodeString(expected); err != nil {
		_ = os.Remove(archive)
		return "", fmt.Errorf("published checksum for %s is not a SHA-256 digest", filepath.Base(archiveURL))
	}
	actual, err := fileSHA256(archive)
	if err != nil {
		_ = os.Remove(archive)
		return "", err
	}
	if !strings.EqualFold(actual, expected) {
		_ = os.Remove(archive)
		return "", fmt.Errorf("checksum mismatch for %s: refusing to install", filepath.Base(archiveURL))
	}
	return archive, nil
}

// fetch streams url into a new temporary file and returns its path.
//
// report, when non-nil, receives the running byte count and the response's
// Content-Length (-1 when the server does not send one) after every chunk, so a
// caller can render progress. It must be cheap and must not block.
func (o Options) fetch(ctx context.Context, url string, report func(done, total int64), maxBytes int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := o.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s answered %s", url, resp.Status)
	}
	file, err := os.CreateTemp("", ".ompig-slim-download-*")
	if err != nil {
		return "", fmt.Errorf("stage download: %w", err)
	}
	name := file.Name()
	// Content-Length bounds the body; ContentLength is -1 when the server omits
	// it, which the meter reads as an unknown total.
	total := resp.ContentLength
	if report != nil {
		report(0, total)
	}
	body := io.Reader(resp.Body)
	if report != nil {
		body = &countingReader{inner: resp.Body, total: total, report: report}
	}
	written, copyErr := io.Copy(file, io.LimitReader(body, maxBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(name)
		return "", errors.Join(copyErr, closeErr)
	}
	if written > maxBytes {
		_ = os.Remove(name)
		return "", fmt.Errorf("%s exceeds the %d-byte limit", url, maxBytes)
	}
	return name, nil
}

func (o Options) replace(ctx context.Context, archive string, artifact releaseArtifact) error {
	staged, err := extractExecutable(archive, filepath.Dir(o.ExePath), artifact)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(staged) }()
	if err := exec.CommandContext(ctx, staged, StageReplacementArg).Run(); err != nil {
		return fmt.Errorf("prepare replacement SDKs: %w", err)
	}
	return commitReplacement(staged, o.ExePath)
}

// isNewer reports whether candidate is a later release than current.
//
// Both are already stripped of a leading "v", and ordering is semver's: a
// prerelease sorts before its release, and prerelease identifiers compare
// numerically where they are numeric (so -alpha.10 is newer than -alpha.9).
// When either side is not a semantic version, any difference counts as newer,
// so an odd tag still offers an update instead of silently reporting success.
func isNewer(candidate, current string) bool {
	left, leftErr := semver.StrictNewVersion(candidate)
	right, rightErr := semver.StrictNewVersion(current)
	if leftErr != nil || rightErr != nil {
		return candidate != current
	}
	return left.Compare(right) > 0
}

// countingReader reports byte progress while a response body streams past. It
// forwards reads unchanged, so the caller's size limit and hashing still see the
// same bytes.
type countingReader struct {
	inner  io.Reader
	total  int64
	done   int64
	report func(done, total int64)
}

// Read forwards bytes and reports the running total. It returns read errors unchanged.
func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.inner.Read(p)
	if n > 0 {
		c.done += int64(n)
		c.report(c.done, c.total)
	}
	return n, err
}

// fileSHA256 returns the lowercase hex SHA-256 of the file at name.
func fileSHA256(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	_, copyErr := io.Copy(digest, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return "", errors.Join(copyErr, closeErr)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// request contains the self-update options extracted from an update invocation.
type request struct {
	all     bool
	force   bool
	channel string
}

// resolveRequest reads a `update`/`upgrade` argument list without running
// anything or touching the filesystem. It reports self=false for every form the
// package manager, the model catalog, or the help text owns, so the caller
// delegates those to PiG unchanged.
//
// It mirrors PiG's own package parser only far enough to tell those apart. An
// explicit self target combined with --extensions means --all, as in PiG.
func resolveRequest(args []string) (resolved request, ok bool) {
	if len(args) == 0 || (args[0] != "update" && args[0] != "upgrade") {
		return request{}, false
	}
	selfTarget, extensionsOnly := false, false
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--channel" {
			if i+1 == len(args) {
				resolved.channel = "invalid"
				continue
			}
			i++
			arg = "--channel=" + args[i]
		}
		if channel, found := strings.CutPrefix(arg, "--channel="); found {
			resolved.channel = channel
			if resolved.channel == "" {
				resolved.channel = "invalid"
			}
			continue
		}
		switch arg {
		case "-h", "--help", "--models", "--extension":
			return request{}, false
		case "--all":
			resolved.all = true
		case "--force":
			resolved.force = true
		case "--extensions":
			extensionsOnly = true
		case "--self", "self", "ompig-slim", "pig":
			selfTarget = true
		case "-a", "--approve", "-na", "--no-approve":
		default:
			// A positional package source or an unknown flag: the package
			// manager owns this request, including its own error for it.
			return request{}, false
		}
	}
	if extensionsOnly {
		if !selfTarget {
			// `update --extensions` alone is the package-only form.
			return request{}, false
		}
		// PiG resolves an explicit self target combined with --extensions to
		// --all: packages first, then this binary.
		resolved.all = true
	}
	return resolved, true
}

// Handle intercepts self-update forms of update and upgrade. Unhandled forms
// return arguments for PiG, with upgrade rewritten to update.
func Handle(args []string, version string, stdout, stderr io.Writer) (delegated []string, handled bool, code int) {
	resolved, self := resolveRequest(args)
	if !self {
		return canonicalUpdate(args), false, 0
	}

	if resolved.channel != "" && resolved.channel != "stable" && resolved.channel != "beta" {
		fmt.Fprintln(stderr, "Update failed: --channel must be stable or beta")
		return nil, true, 2
	}

	marker, err := readMarker()
	if err != nil {
		return canonicalUpdate(args), false, 0
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "Update failed: cannot locate the running executable: %v\n", err)
		return nil, true, 1
	}
	// A build running beside the marked installation is not allowed to replace it.
	target, ok := ownedExecutable(marker.ExecutablePath, executable)
	if !ok {
		return canonicalUpdate(args), false, 0
	}
	if resolved.all {
		if childCode := runPackageUpdate(executable, stdout, stderr); childCode != 0 {
			return nil, true, childCode
		}
	}

	unlock, err := lockUpdate(target)
	if err != nil {
		fmt.Fprintf(stderr, "Update failed: %v\n", err)
		return nil, true, 1
	}
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()
	marker, err = readMarker()
	if err != nil {
		fmt.Fprintf(stderr, "Update failed: could not re-read the install marker: %v\n", err)
		return nil, true, 1
	}
	currentTarget, ok := ownedExecutable(marker.ExecutablePath, executable)
	if !ok || !sameUpdatePath(target, currentTarget) {
		fmt.Fprintln(stderr, "Update failed: the installed executable changed; restart and retry")
		return nil, true, 1
	}
	liveVersion, err := readInstalledVersion(ctx, currentTarget)
	if err != nil {
		fmt.Fprintf(stderr, "Update failed: cannot verify the installed executable: %v\n", err)
		return nil, true, 1
	}
	currentVersion := strings.TrimPrefix(strings.TrimSpace(version), "v")
	liveVersion = strings.TrimPrefix(strings.TrimSpace(liveVersion), "v")
	if currentVersion != liveVersion {
		fmt.Fprintf(stderr, "Update failed: this process runs ompig-slim %s, but %s is installed; restart and retry\n", currentVersion, liveVersion)
		return nil, true, 1
	}

	channel := resolved.channel
	if channel == "" {
		channel = marker.Channel
	}
	downloadBase := strings.TrimSpace(os.Getenv("OMPIG_SLIM_DOWNLOAD_BASE"))
	if downloadBase == "" {
		downloadBase = marker.DownloadBase
	}
	if downloadBase == "" {
		downloadBase = defaultDownloadBase
	}
	releasesAPI := strings.TrimSpace(os.Getenv("OMPIG_SLIM_RELEASES_API"))
	if releasesAPI == "" {
		releasesAPI = defaultReleasesAPI
	}
	var savePreference func() error
	if resolved.channel != "" && resolved.channel != marker.Channel {
		requestedChannel := resolved.channel
		savePreference = func() error { return saveChannel(requestedChannel) }
	}
	return nil, true, updateLocked(ctx, Options{
		Version:      liveVersion,
		ExePath:      currentTarget,
		DownloadBase: downloadBase,
		ReleasesAPI:  releasesAPI,
		Stdout:       stdout,
		Stderr:       stderr,
		Force:        resolved.force,
		Channel:      channel,
	}, savePreference)
}

func readInstalledVersion(ctx context.Context, executable string) (string, error) {
	command := exec.CommandContext(ctx, executable, "--version")
	environment := os.Environ()
	command.Env = make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "PIG_VERSION_FORMAT") {
			command.Env = append(command.Env, entry)
		}
	}
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", errors.New("installed executable returned an empty version")
	}
	return version, nil
}

func sameUpdatePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// canonicalUpdate returns args with the `upgrade` alias rewritten to `update`.
// PiG has no upgrade command, so passing the alias through would make PiG read
// the word as a prompt instead of a command. The input slice is not modified.
func canonicalUpdate(args []string) []string {
	if len(args) == 0 || args[0] != "upgrade" {
		return args
	}
	delegated := make([]string, len(args))
	copy(delegated, args)
	delegated[0] = "update"
	return delegated
}

// runPackageUpdate runs the package-manager phase of `update --all` through a
// child process, because PiG's dispatch ends the process it runs in.
func runPackageUpdate(executable string, stdout, stderr io.Writer) int {
	command := exec.Command(executable, "update", "--extensions")
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		fmt.Fprintf(stderr, "Update failed: installed packages: %v\n", err)
		return 1
	}
	return 0
}

// marker is what scripts/install.sh records about an archive installation.
type marker struct {
	// ExecutablePath is the installed file the marker was written for. Update
	// only replaces this path, so a different build run beside an installed copy
	// is never overwritten by a release archive.
	ExecutablePath string
	// DownloadBase is the release root the installer used, so later updates
	// come from the same place.
	DownloadBase string
	// Channel defaults to stable for markers written before channel selection.
	Channel string
}

// configRoot mirrors PiG's own resolution, which internal/configroot owns so the
// installer marker and the published agent state cannot disagree about where the
// configuration root is.
func configRoot() (string, error) { return configroot.Dir() }

// readMarker reads the archive-install marker written by scripts/install.sh.
// A missing, unreadable, or foreign marker returns an error, which callers read
// as "this package does not own the installation".
func readMarker() (marker, error) {
	root, err := configRoot()
	if err != nil {
		return marker{}, err
	}
	path := filepath.Join(root, markerName)
	data, err := os.ReadFile(path)
	if err != nil {
		return marker{}, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found || key == "" {
			continue
		}
		values[key] = value
	}
	if values["kind"] != markerKind {
		return marker{}, fmt.Errorf("%s does not describe an archive installation", path)
	}
	channel := values["channel"]
	if channel == "" {
		channel = "stable"
	}
	return marker{
		ExecutablePath: values["executable"],
		DownloadBase:   values["download-base"],
		Channel:        channel,
	}, nil
}

// saveChannel atomically rewrites only the channel in the installer marker,
// preserving ownership and mirror metadata. Call only after a successful
// update (including a no-op); failed downloads must retain the old preference.
func saveChannel(channel string) error {
	root, err := configRoot()
	if err != nil {
		return err
	}
	name := filepath.Join(root, markerName)
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		key, _, _ := strings.Cut(strings.TrimSpace(line), "=")
		if key != "channel" {
			lines = append(lines, line)
		}
	}
	lines = append(lines, "channel="+channel)
	staged, err := os.CreateTemp(root, ".ompig-slim-install-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(staged.Name()) }()
	_, writeErr := io.WriteString(staged, strings.Join(lines, "\n")+"\n")
	closeErr := staged.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(staged.Name(), name)
}
