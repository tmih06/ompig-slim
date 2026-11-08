// Package selfupdate replaces a curl-installed ompig-slim with a newer release.
//
// The vendored PiG self-updater only mutates an installation it can prove it
// owns: a package-manager install, or a standalone binary with a signed update
// manifest and an installation receipt. This repository publishes neither a
// signed manifest nor a trust root, so on a binary that scripts/install.sh
// placed in ~/.local/bin every `ompig-slim update` refused. This package closes
// that gap using the same trust decision the installer already makes: the
// release archive plus its published SHA-256 sidecar, downloaded from the
// release the installer recorded.
//
// Ownership is explicit. scripts/install.sh writes a marker under the config
// root; without that marker this package declines, so an npm, go-install, or
// container installation keeps PiG's own refusal and remediation instead of
// being silently replaced by a GitHub archive.
package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// updateTimeout bounds the whole operation. A release archive is roughly
	// 32 MB, so this is generous while still failing a hung connection.
	updateTimeout = 5 * time.Minute

	// maxArchiveBytes bounds both the download and the extracted executable.
	// It matches PiG's own self-update limit so neither path accepts more.
	maxArchiveBytes = 512 << 20

	defaultDownloadBase = "https://github.com/tmih06/ompig-slim/releases"
	defaultReleasesAPI  = "https://api.github.com/repos/tmih06/ompig-slim/releases/latest"

	// markerName is the file scripts/install.sh writes under the config root,
	// and archiveMember is the executable's path inside a release archive.
	markerName    = "ompig-slim-install"
	archiveMember = "ompig-slim/ompig-slim"

	// markerKind values distinguish an archive install from anything else.
	markerKind = "archive"
)

// Options is everything Update needs, so tests can drive it without touching
// the running executable or the real release host.
type Options struct {
	// Version is the running release, such as "0.1.0"; a leading "v" is
	// ignored. It is compared against the release tag.
	Version string
	// ExePath is the file to replace. It must be the running executable in
	// production: Update replaces it by renaming over it.
	ExePath string
	// DownloadBase is the release root that serves
	// download/<tag>/ompig-slim-<platform>.tar.gz, mirroring GitHub's layout.
	DownloadBase string
	// ReleasesAPI returns the latest non-prerelease release document.
	ReleasesAPI string
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

// Update downloads the latest verified release and atomically replaces
// Options.ExePath with it. It returns a process exit status: 0 when the
// executable is current or was replaced, 1 when anything failed before the
// rename, which leaves the previous executable untouched.
func Update(o Options) int {
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: updateTimeout}
	}
	if strings.TrimSpace(o.ExePath) == "" {
		fmt.Fprintln(o.Stderr, "Update failed: the running executable could not be located")
		return 1
	}

	archiveName, err := archiveNameFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		fmt.Fprintf(o.Stderr, "Update failed: %v\n", err)
		return 1
	}
	current := strings.TrimPrefix(strings.TrimSpace(o.Version), "v")

	tag, err := o.latestTag()
	if err != nil {
		fmt.Fprintf(o.Stderr, "Update failed: could not reach the release channel: %v\n%s\n", err, fallback())
		return 1
	}
	latest := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if !o.Force && !isNewer(latest, current) {
		fmt.Fprintf(o.Stdout, "ompig-slim %s is up to date.\n", current)
		return 0
	}

	archiveURL := strings.TrimRight(o.DownloadBase, "/") + "/download/" + tag + "/" + archiveName
	fmt.Fprintf(o.Stdout, "Updating ompig-slim %s → %s...\n", current, latest)
	archive, err := o.download(archiveURL)
	if err != nil {
		fmt.Fprintf(o.Stderr, "Update failed: %v\n%s\n", err, fallback())
		return 1
	}
	defer func() { _ = os.Remove(archive) }()

	if err := o.replace(archive); err != nil {
		fmt.Fprintf(o.Stderr, "Update failed: %v\n%s\n", err, fallback())
		return 1
	}

	fmt.Fprintf(o.Stdout, "Updated to ompig-slim %s.\n", latest)
	// The replaced binary ships its own embedded SDK, which only it can stage.
	// This process still holds the previous bytes, so saying nothing would make
	// the slower first launch look like a hang.
	fmt.Fprintln(o.Stdout, "Source extensions rebuild on next start; the first launch will be slower.")
	return 0
}

// fallback is the instruction that always works when an update cannot finish.
func fallback() string {
	return "Reinstall with the installer instead: curl -fsSL " +
		"https://raw.githubusercontent.com/tmih06/ompig-slim/main/scripts/install.sh | bash"
}

// latestTag returns the tag of the latest published release.
func (o Options) latestTag() (string, error) {
	req, err := http.NewRequest(http.MethodGet, o.ReleasesAPI, nil)
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
		TagName string `json:"tag_name"`
	}
	// Bound the read: a release document is small, and this endpoint is remote.
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&document); err != nil {
		return "", fmt.Errorf("parse release channel response: %w", err)
	}
	tag := strings.TrimSpace(document.TagName)
	if tag == "" {
		return "", errors.New("release channel did not name a release")
	}
	return tag, nil
}

// download fetches the archive and its published SHA-256 into the system
// temporary directory, verifying the digest before returning a path to the
// verified archive. The caller owns that file.
//
// The checksum comes from the same release as the archive, exactly as in
// scripts/install.sh: it detects a corrupt, truncated, or swapped download. It
// is not a signature and does not defend against a compromised release.
func (o Options) download(archiveURL string) (string, error) {
	archive, err := o.fetch(archiveURL)
	if err != nil {
		return "", err
	}
	checksumFile, err := o.fetch(archiveURL + ".sha256")
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
	expected := strings.TrimSpace(strings.Fields(string(raw))[0])
	if len(expected) != sha256.Size*2 {
		_ = os.Remove(archive)
		return "", fmt.Errorf("published checksum for %s is not a SHA-256 digest", filepath.Base(archiveURL))
	}
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
func (o Options) fetch(url string) (string, error) {
	resp, err := o.Client.Get(url)
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
	written, copyErr := io.Copy(file, io.LimitReader(resp.Body, maxArchiveBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(name)
		return "", errors.Join(copyErr, closeErr)
	}
	if written > maxArchiveBytes {
		_ = os.Remove(name)
		return "", fmt.Errorf("%s exceeds the %d-byte limit", url, int64(maxArchiveBytes))
	}
	return name, nil
}

// replace extracts the executable from the archive into a temporary file beside
// Options.ExePath and renames it into place.
//
// Staging beside the destination keeps the rename atomic on one filesystem, so
// a running instance keeps its old inode and a failed copy never truncates the
// installed binary.
func (o Options) replace(archive string) error {
	dir := filepath.Dir(o.ExePath)
	staged, err := extractExecutable(archive, dir)
	if err != nil {
		return err
	}
	// From here the staged file is either renamed or removed; never leave it
	// behind, because it sits in a directory the user has on PATH.
	renameErr := os.Rename(staged, o.ExePath)
	if renameErr != nil {
		_ = os.Remove(staged)
	}
	return renameErr
}

// extractExecutable writes the archive's single ompig-slim executable to a new
// temporary file in dir and returns its path.
func extractExecutable(archive, dir string) (string, error) {
	file, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	uncompressed, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("read release archive: %w", err)
	}
	defer func() { _ = uncompressed.Close() }()

	reader := tar.NewReader(uncompressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read release archive: %w", err)
		}
		if path.Clean(header.Name) != archiveMember {
			continue
		}
		// Only a regular file is acceptable: a directory or an entry that links
		// elsewhere would put something other than the released program on PATH.
		if header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maxArchiveBytes {
			return "", fmt.Errorf("%s in the release archive is not a regular file", archiveMember)
		}
		staged, err := os.CreateTemp(dir, ".ompig-slim-update-*")
		if err != nil {
			return "", fmt.Errorf("stage update (is %s writable?): %w", dir, err)
		}
		name := staged.Name()
		written, copyErr := io.Copy(staged, io.LimitReader(reader, header.Size))
		closeErr := staged.Close()
		if copyErr != nil || closeErr != nil || written != header.Size {
			_ = os.Remove(name)
			return "", errors.Join(copyErr, closeErr, fmt.Errorf("copy %s: wrote %d of %d bytes", archiveMember, written, header.Size))
		}
		if err := os.Chmod(name, 0o755); err != nil {
			_ = os.Remove(name)
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("release archive does not contain %s", archiveMember)
}

// archiveNameFor maps a Go platform to the release archive that serves it. The
// four names here are exactly the ones scripts/build-binaries.sh publishes.
func archiveNameFor(goos, goarch string) (string, error) {
	var arch string
	switch goarch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("no release archive for CPU %s (supported: x86_64, arm64)", goarch)
	}
	switch goos {
	case "linux", "darwin":
	default:
		return "", fmt.Errorf("no release archive for %s (supported: Linux, macOS)", goos)
	}
	return "ompig-slim-" + goos + "-" + arch + ".tar.gz", nil
}

// isNewer reports whether candidate is a later release than current. Both are
// already stripped of a leading "v". A prerelease sorts before its release, and
// an unparsable pair falls back to inequality so an odd tag still offers an
// update rather than silently reporting success.
func isNewer(candidate, current string) bool {
	if order, ok := compareVersions(candidate, current); ok {
		return order > 0
	}
	return candidate != current
}

// compareVersions orders two semantic versions, reporting false when either is
// not of the form major.minor.patch with an optional prerelease suffix.
func compareVersions(left, right string) (int, bool) {
	l, lpre, ok := parseVersion(left)
	if !ok {
		return 0, false
	}
	r, rpre, ok := parseVersion(right)
	if !ok {
		return 0, false
	}
	for i := range l {
		if l[i] != r[i] {
			if l[i] > r[i] {
				return 1, true
			}
			return -1, true
		}
	}
	// Build metadata does not order releases; only the prerelease suffix does.
	switch {
	case lpre == rpre:
		return 0, true
	case lpre == "":
		return 1, true
	case rpre == "":
		return -1, true
	case lpre < rpre:
		return -1, true
	default:
		return 1, true
	}
}

// parseVersion splits major.minor.patch from an optional prerelease suffix.
func parseVersion(raw string) ([3]int, string, bool) {
	var numbers [3]int
	core := raw
	prerelease := ""
	if before, after, found := strings.Cut(raw, "-"); found {
		core, prerelease = before, after
	}
	// Drop build metadata; it never affects ordering.
	if before, _, found := strings.Cut(core, "+"); found {
		core = before
	}
	if before, _, found := strings.Cut(prerelease, "+"); found {
		prerelease = before
	}
	parts := strings.Split(core, ".")
	if len(parts) != len(numbers) {
		return numbers, "", false
	}
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return numbers, "", false
		}
		numbers[i] = value
	}
	return numbers, prerelease, true
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

// request is a parsed `update`/`upgrade` invocation.
type request struct {
	// self is true when the caller asked for this binary, by the bare command or
	// by naming it explicitly.
	self bool
	// all is true when packages are updated before the binary.
	all bool
	// force reinstalls even when the running version is current.
	force bool
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
	for _, arg := range args[1:] {
		switch arg {
		case "-h", "--help", "--models", "--extension":
			// Package and help forms are PiG's. --extension is followed by a
			// source value that must not be inspected or reordered here.
			return request{}, false
		case "--all":
			resolved.all = true
		case "--force":
			resolved.force = true
		case "--extensions":
			extensionsOnly = true
		case "--self", "self", "ompig-slim", "pig":
			// Every spelling of the binary's own name is the self-update.
			selfTarget = true
		case "-a", "--approve", "-na", "--no-approve":
			// Project-trust flags travel with a package operation; alone they
			// leave the default self-update, as in PiG.
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
	resolved.self = true
	return resolved, true
}

// Handle claims the self-update forms of `update` and `upgrade`.
//
// It returns the arguments to pass on to PiG. When handled is true the caller
// must exit with code and ignore the returned arguments; otherwise it must run
// the returned arguments, which may have `upgrade` rewritten to `update`.
//
// `upgrade` is a second spelling of `update`, so every form this package does
// not own is delegated under the canonical name. Only a self-update on a
// marker-bearing archive install is claimed; package, model, and extension
// updates keep going to PiG, as does the refusal for any other install method.
func Handle(args []string, version string, stdout, stderr io.Writer) (delegated []string, handled bool, code int) {
	resolved, self := resolveRequest(args)
	if !self {
		return canonicalUpdate(args), false, 0
	}

	marker, err := readMarker()
	if err != nil {
		// Not an archive install, so this package must not replace the binary.
		// PiG names the install method and the correct way to update it.
		return canonicalUpdate(args), false, 0
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "Update failed: cannot locate the running executable: %v\n", err)
		return nil, true, 1
	}
	// The marker names the one installed file it describes. A different build
	// running beside that installation (a repository build, or a binary copied
	// elsewhere) is not this package's to replace, so PiG keeps the decision.
	if !sameExecutable(marker.ExecutablePath, executable) {
		return canonicalUpdate(args), false, 0
	}
	if resolved.all {
		// `update --all` is packages then this binary, matching PiG's order. The
		// child runs the same executable with the package-only form, which this
		// function delegates, so the recursion stops after one level.
		if childCode := runPackageUpdate(executable, stdout, stderr); childCode != 0 {
			return nil, true, childCode
		}
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
	return nil, true, Update(Options{
		Version:      version,
		ExePath:      executable,
		DownloadBase: downloadBase,
		ReleasesAPI:  releasesAPI,
		Stdout:       stdout,
		Stderr:       stderr,
		Force:        resolved.force,
	})
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
}

// configRoot mirrors PiG's own resolution: PIG_HOME when set, else ~/.pi.
func configRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("PIG_HOME")); root != "" {
		return root, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pi"), nil
}

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
	return marker{
		ExecutablePath: values["executable"],
		DownloadBase:   values["download-base"],
	}, nil
}

// sameExecutable reports whether two spellings name the same installed file.
// Both are reduced through symlinks first, so a symlink on PATH that points at
// the installed executable still matches the marker. When a path cannot be
// resolved it is compared as written, which is the conservative answer: an
// unresolvable running executable was already rejected by the caller.
func sameExecutable(marked, running string) bool {
	if marked == "" || running == "" {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(marked); err == nil {
		marked = resolved
	}
	if resolved, err := filepath.EvalSymlinks(running); err == nil {
		running = resolved
	}
	markedAbs, markedErr := filepath.Abs(marked)
	runningAbs, runningErr := filepath.Abs(running)
	if markedErr != nil || runningErr != nil {
		return marked == running
	}
	return markedAbs == runningAbs
}
