package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "update" && os.Args[2] == "--extensions" {
		if started := os.Getenv("OMPIG_TEST_PACKAGE_STARTED"); started != "" {
			_ = os.WriteFile(started, []byte("ready"), 0o600)
		}
		gate := os.Getenv("OMPIG_TEST_PACKAGE_GATE")
		deadline := time.Now().Add(time.Minute)
		for gate != "" {
			if _, err := os.Stat(gate); err == nil {
				os.Exit(0)
			}
			if time.Now().After(deadline) {
				os.Exit(41)
			}
			time.Sleep(10 * time.Millisecond)
		}
		os.Exit(0)
	}
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case StageReplacementArg:
			if os.Getenv("OMPIG_TEST_STAGE_FAILURE") == "1" {
				os.Exit(42)
			}
			if os.Getenv("OMPIG_TEST_STAGE_WAIT") == "1" {
				time.Sleep(time.Minute)
			}
			os.Exit(0)
		case "--fixture-self-update":
			_, handled, code := Handle([]string{"upgrade", "--channel=stable"}, "3.0.0-beta.1", os.Stdout, os.Stderr)
			if !handled {
				fmt.Fprintln(os.Stderr, "running executable did not match the installer marker")
				os.Exit(2)
			}
			os.Exit(code)
		case "--fixture-stale-default", "--fixture-stale-version":
			version := "2.0.0"
			if os.Args[1] == "--fixture-stale-version" {
				version = "1.0.0"
			}
			_, handled, code := Handle([]string{"update", "--all"}, version, os.Stdout, os.Stderr)
			if !handled {
				fmt.Fprintln(os.Stderr, "running executable did not match the installer marker")
				os.Exit(2)
			}
			os.Exit(code)
		case "--version":
			version := os.Getenv("OMPIG_TEST_VERSION")
			if version == "" {
				version = "2.0.0"
			}
			fmt.Println(version)
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func fixturePayload(t *testing.T, _ string) []byte {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestWindowsRunningExecutableUpdatesItself(t *testing.T) {
	artifact := nativeArtifact(t)
	payload := fixturePayload(t, "")
	archive := nativeArchive(t, artifact.member, payload)
	server := releaseServer(t, "v2.0.0", archive, digestOf(archive))
	dir := filepath.Join(t.TempDir(), "Native install é")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "ompig-slim.exe")
	before := append(bytes.Clone(payload), []byte("old image")...)
	if err := os.WriteFile(target, before, 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("PIG_HOME", root)
	t.Setenv("OMPIG_TEST_VERSION", "3.0.0-beta.1")
	t.Setenv("OMPIG_SLIM_DOWNLOAD_BASE", "")
	t.Setenv("OMPIG_SLIM_RELEASES_API", server.URL+"/releases/latest")
	marker := "kind=archive\r\nexecutable=" + filepath.Join(dir, "OMPIG-SLIM.EXE") + "\r\ndownload-base=" + server.URL + "/releases\r\nchannel=beta\r\n"
	if err := os.WriteFile(filepath.Join(root, markerName), []byte(marker), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, target, "--fixture-self-update").CombinedOutput()
	if err != nil || !bytes.Contains(out, []byte("Updated ompig-slim 3.0.0-beta.1")) {
		t.Fatalf("self-update failed: %v\n%s", err, out)
	}
	assertFileBytes(t, target, payload)
	assertFileBytes(t, filepath.Join(dir, "ompig-slim.previous.exe"), before)
	t.Setenv("OMPIG_TEST_VERSION", "")
	out, err = exec.CommandContext(ctx, target, "--version").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "2.0.0" {
		t.Fatalf("updated executable did not run: %v\n%s", err, out)
	}
	saved, err := readMarker()
	if err != nil || saved.Channel != "stable" || saved.DownloadBase != server.URL+"/releases" {
		t.Fatalf("unexpected marker after update: %+v, %v", saved, err)
	}
}

func TestWindowsHandleRechecksMarkerAndRejectsStaleProcess(t *testing.T) {
	artifact := nativeArtifact(t)
	payload := fixturePayload(t, "")
	archive := nativeArchive(t, artifact.member, payload)
	for _, tc := range []struct {
		name, mode string
		stale      bool
	}{
		{name: "fresh preference", mode: "--fixture-stale-default"},
		{name: "different installed version", mode: "--fixture-stale-version", stale: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var betaRequests, downloads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/releases/latest":
					_, _ = fmt.Fprint(w, `{"tag_name":"v2.0.0"}`)
				case r.URL.Path == "/releases":
					betaRequests.Add(1)
					_, _ = fmt.Fprint(w, `[{"tag_name":"v9.0.0-beta.1","prerelease":true}]`)
				case strings.HasSuffix(r.URL.Path, ".sha256"):
					_, _ = fmt.Fprint(w, digestOf(archive))
				case strings.Contains(r.URL.Path, "/download/"):
					downloads.Add(1)
					_, _ = w.Write(archive)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			dir, root := t.TempDir(), t.TempDir()
			target := filepath.Join(dir, "ompig-slim.exe")
			if err := os.WriteFile(target, payload, 0o755); err != nil {
				t.Fatal(err)
			}
			marker := "kind=archive\nexecutable=" + target + "\ndownload-base=" + server.URL + "/releases\nchannel=beta\n"
			if err := os.WriteFile(filepath.Join(root, markerName), []byte(marker), 0o600); err != nil {
				t.Fatal(err)
			}
			gate, started := filepath.Join(t.TempDir(), "gate"), filepath.Join(t.TempDir(), "started")
			t.Cleanup(func() { _ = os.WriteFile(gate, []byte("continue"), 0o600) })
			t.Setenv("PIG_HOME", root)
			t.Setenv("OMPIG_TEST_PACKAGE_GATE", gate)
			t.Setenv("OMPIG_TEST_PACKAGE_STARTED", started)
			t.Setenv("OMPIG_SLIM_DOWNLOAD_BASE", "")
			t.Setenv("OMPIG_SLIM_RELEASES_API", server.URL+"/releases/latest")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, target, tc.mode)
			var output bytes.Buffer
			command.Stdout, command.Stderr = &output, &output
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, err := os.Stat(started); err == nil {
					break
				}
				if time.Now().After(deadline) {
					_ = command.Process.Kill()
					_ = command.Wait()
					t.Fatal("package update did not reach its wait point")
				}
				time.Sleep(10 * time.Millisecond)
			}

			oldFile, err := os.Open(target)
			if err != nil {
				t.Fatal(err)
			}
			before, statErr := oldFile.Stat()
			closeErr := oldFile.Close()
			if statErr != nil || closeErr != nil {
				t.Fatalf("capture old file identity: stat=%v, close=%v", statErr, closeErr)
			}
			unlock, err := lockUpdate(target)
			if err != nil {
				t.Fatal(err)
			}
			staged := filepath.Join(dir, "replacement.exe")
			if err := os.WriteFile(staged, payload, 0o755); err != nil {
				unlock()
				t.Fatal(err)
			}
			if err := commitReplacement(staged, target); err != nil {
				unlock()
				t.Fatal(err)
			}
			if err := saveChannel("stable"); err != nil {
				unlock()
				t.Fatal(err)
			}
			unlock()
			after, err := os.Stat(target)
			if err != nil || os.SameFile(before, after) {
				t.Fatalf("replacement did not install a new file identity: %v", err)
			}
			if err := os.WriteFile(gate, []byte("continue"), 0o600); err != nil {
				t.Fatal(err)
			}
			err = command.Wait()
			if tc.stale {
				if err == nil || !strings.Contains(output.String(), "1.0.0, but 2.0.0 is installed") {
					t.Fatalf("stale process result = %v, output = %q", err, output.String())
				}
			} else if err != nil || !strings.Contains(output.String(), "2.0.0 is up to date") {
				t.Fatalf("update result = %v, output = %q", err, output.String())
			}
			if betaRequests.Load() != 0 || downloads.Load() != 0 {
				t.Fatalf("stale marker selected beta or downloaded an archive: beta requests=%d downloads=%d", betaRequests.Load(), downloads.Load())
			}
			saved, err := readMarker()
			if err != nil || saved.Channel != "stable" {
				t.Fatalf("marker channel = %q, error = %v", saved.Channel, err)
			}
		})
	}
}

func windowsLongPathDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for len(filepath.Join(dir, "ompig-slim.exe")) <= 300 {
		dir = filepath.Join(dir, "long-path-component-1234567890")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestWindowsLongPathUpdateAndRollback(t *testing.T) {
	artifact := nativeArtifact(t)
	payload := fixturePayload(t, "")
	archive := nativeArchive(t, artifact.member, payload)
	server := releaseServer(t, "v2.0.0", archive, digestOf(archive))
	dir := windowsLongPathDir(t)
	target := filepath.Join(dir, "ompig-slim.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := ownedExecutable(target, target); !ok {
		t.Fatal("ownership rejected a long absolute path")
	}
	var stderr bytes.Buffer
	if code := Update(Options{Version: "1.0.0", ExePath: target,
		DownloadBase: server.URL + "/releases", ReleasesAPI: server.URL + "/releases/latest",
		Client: server.Client(), Stderr: &stderr}); code != 0 {
		t.Fatalf("Update = %d, stderr = %q", code, &stderr)
	}
	assertFileBytes(t, target, payload)
	assertFileBytes(t, previousExecutable(target), []byte("old"))

	rollbackDir := windowsLongPathDir(t)
	rollbackTarget := filepath.Join(rollbackDir, "ompig-slim.exe")
	if err := os.WriteFile(rollbackTarget, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(rollbackDir, "missing.exe")
	if err := commitReplacement(missing, rollbackTarget); err == nil || !strings.Contains(err.Error(), "install staged") {
		t.Fatalf("expected long-path staged move failure, got %v", err)
	}
	assertFileBytes(t, rollbackTarget, []byte("old"))
	if _, err := os.Stat(previousExecutable(rollbackTarget)); !os.IsNotExist(err) {
		t.Fatalf("long-path rollback did not restore the backup: %v", err)
	}
}

func TestWindowsUpdateDeadlineStopsSDKStaging(t *testing.T) {
	t.Setenv("OMPIG_TEST_STAGE_WAIT", "1")
	artifact := nativeArtifact(t)
	archive := nativeArchive(t, artifact.member, fixturePayload(t, ""))
	server := releaseServer(t, "v2.0.0", archive, digestOf(archive))
	target := filepath.Join(t.TempDir(), "ompig-slim.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := updateWithTimeout(Options{Version: "1.0.0", ExePath: target,
		DownloadBase: server.URL + "/releases", ReleasesAPI: server.URL + "/releases/latest",
		Client: server.Client(), Stderr: &stderr}, time.Second)
	if code != 1 || !strings.Contains(stderr.String(), "prepare replacement SDKs") {
		t.Fatalf("Update = %d, stderr = %q", code, &stderr)
	}
	assertFileBytes(t, target, []byte("old"))
	unlock, err := lockUpdate(target)
	if err != nil {
		t.Fatalf("update lock remained held after the deadline: %v", err)
	}
	unlock()
}

func TestWindowsFileMutationRetriesOnlyTransientConflicts(t *testing.T) {
	for _, conflict := range []error{windows.ERROR_SHARING_VIOLATION, windows.ERROR_LOCK_VIOLATION} {
		t.Run(conflict.Error(), func(t *testing.T) {
			attempts := 0
			err := retryWindowsFileMutation(func() error {
				attempts++
				if attempts == 1 {
					return &os.PathError{Op: "rename", Path: "staged.exe", Err: conflict}
				}
				return nil
			})
			if err != nil || attempts != 2 {
				t.Fatalf("transient mutation = %v after %d attempts", err, attempts)
			}
		})
	}
	for _, failure := range []error{windows.ERROR_ACCESS_DENIED, windows.ERROR_ALREADY_EXISTS, windows.ERROR_FILE_NOT_FOUND} {
		attempts := 0
		err := retryWindowsFileMutation(func() error {
			attempts++
			return failure
		})
		if !errors.Is(err, failure) || attempts != 1 {
			t.Fatalf("permanent mutation = %v after %d attempts, want %v", err, attempts, failure)
		}
	}
}

func TestWindowsReplacementWaitsForTransientFileHandles(t *testing.T) {
	for _, busy := range []string{"staged.exe", "ompig-slim.exe", "ompig-slim.previous.exe"} {
		t.Run(busy, func(t *testing.T) {
			dir := t.TempDir()
			target, staged := filepath.Join(dir, "ompig-slim.exe"), filepath.Join(dir, "staged.exe")
			for name, data := range map[string]string{target: "old", staged: "new", previousExecutable(target): "previous"} {
				if err := os.WriteFile(name, []byte(data), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			name, err := windows.UTF16PtrFromString(filepath.Join(dir, busy))
			if err != nil {
				t.Fatal(err)
			}
			handle, err := windows.CreateFile(name, windows.GENERIC_READ,
				windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			released := make(chan error, 1)
			time.AfterFunc(300*time.Millisecond, func() { released <- windows.CloseHandle(handle) })
			err = commitReplacement(staged, target)
			if closeErr := <-released; closeErr != nil {
				t.Fatal(closeErr)
			}
			if err != nil {
				t.Fatalf("replacement failed during a transient file lock: %v", err)
			}
			assertFileBytes(t, target, []byte("new"))
			assertFileBytes(t, previousExecutable(target), []byte("old"))
			if _, err := os.Stat(staged); !os.IsNotExist(err) {
				t.Fatalf("staged executable was not consumed: %v", err)
			}
		})
	}
}

func TestWindowsReplacementRollsBack(t *testing.T) {
	target := filepath.Join(t.TempDir(), "ompig-slim.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := commitReplacement(filepath.Join(filepath.Dir(target), "missing.exe"), target); err == nil || !strings.Contains(err.Error(), "install staged") {
		t.Fatalf("expected staged move failure, got %v", err)
	}
	assertFileBytes(t, target, []byte("old"))
	if _, err := os.Stat(previousExecutable(target)); !os.IsNotExist(err) {
		t.Fatalf("did not restore backup: %v", err)
	}
}

func TestWindowsReplacementPreservesBackupWhenTargetIsMissing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ompig-slim.exe")
	backup := filepath.Join(dir, "ompig-slim.previous.exe")
	staged := filepath.Join(dir, "staged.exe")
	if err := os.WriteFile(backup, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := commitReplacement(staged, target); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q", backup)) {
		t.Fatalf("error did not identify recovery path: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("missing target was unexpectedly created: %v", err)
	}
	assertFileBytes(t, backup, []byte("old"))
	assertFileBytes(t, staged, []byte("new"))
}

func TestWindowsReplacementReportsBlockedRollback(t *testing.T) {
	dir := t.TempDir()
	target, staged := filepath.Join(dir, "ompig-slim.exe"), filepath.Join(dir, "staged.exe")
	for path, data := range map[string]string{target: "old", staged: "new"} {
		if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	err := replaceWindowsExecutable(staged, target, func(from, to string) error {
		if from == staged {
			if err := os.WriteFile(target, []byte("intervening"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return moveWithoutOverwrite(from, to)
	})
	if err == nil || !strings.Contains(err.Error(), "restore failed") || !strings.Contains(err.Error(), "ompig-slim.previous.exe") {
		t.Fatalf("error did not identify recovery path: %v", err)
	}
	assertFileBytes(t, target, []byte("intervening"))
	assertFileBytes(t, previousExecutable(target), []byte("old"))
	assertFileBytes(t, staged, []byte("new"))
}

func TestWindowsReplacementLeavesLiveExecutableWhenBusy(t *testing.T) {
	for _, busy := range []string{"ompig-slim.exe", "ompig-slim.previous.exe"} {
		t.Run(busy, func(t *testing.T) {
			dir := t.TempDir()
			target, staged := filepath.Join(dir, "ompig-slim.exe"), filepath.Join(dir, "staged.exe")
			paths := map[string]string{target: "old", staged: "new"}
			busyPath := filepath.Join(dir, busy)
			if busyPath != target {
				paths[busyPath] = "previous"
			}
			for path, data := range paths {
				if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			name, err := windows.UTF16PtrFromString(busyPath)
			if err != nil {
				t.Fatal(err)
			}
			handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer windows.CloseHandle(handle)
			if err := commitReplacement(staged, target); err == nil {
				t.Fatal("replacement succeeded while a handle denied delete sharing")
			}
			for path, data := range paths {
				assertFileBytes(t, path, []byte(data))
			}
		})
	}
}

func TestWindowsUpdateHonorsStableByteRangeLock(t *testing.T) {
	target := filepath.Join(t.TempDir(), "ompig-slim.exe")
	file, err := os.OpenFile(target+".update.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	handle := windows.Handle(file.Fd())
	var overlapped windows.Overlapped
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlapped); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := Update(Options{ExePath: target, Stderr: &stderr}); code != 1 || !strings.Contains(stderr.String(), "in progress") {
		t.Fatalf("Update = %d, stderr = %q", code, &stderr)
	}
	if err := windows.UnlockFileEx(handle, 0, 1, 0, &overlapped); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockUpdate(target)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	if _, err := os.Stat(file.Name()); err != nil {
		t.Fatalf("stable lock file was removed: %v", err)
	}
}

func TestWindowsOwnershipAcceptsCaseAliasButRejectsOtherHardLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "ompig-slim.exe")
	if err := os.WriteFile(target, []byte("image"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := ownedExecutable(filepath.Join(dir, "OMPIG-SLIM.EXE"), target); !ok {
		t.Fatal("case alias was rejected")
	}
	link := filepath.Join(dir, "another-install.exe")
	if err := os.Link(target, link); err != nil {
		t.Fatal(err)
	}
	if _, ok := ownedExecutable(link, target); ok {
		t.Fatal("a hard link at a different install path was accepted")
	}
}
