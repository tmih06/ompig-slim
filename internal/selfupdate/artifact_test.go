package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func nativeArtifact(t *testing.T) releaseArtifact {
	t.Helper()
	artifact, err := artifactFor(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip("no release archive for this platform")
	}
	return artifact
}

func nativeArchive(t *testing.T, member string, data []byte) []byte {
	t.Helper()
	if runtime.GOOS == "windows" {
		return zipFor(t, []zipEntry{{name: member, data: data}})
	}
	return archiveFor(t, map[string]string{member: string(data)})
}

type zipEntry struct {
	name string
	data []byte
	mode fs.FileMode
	size uint64
}

func zipFor(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name}
		header.SetMode(entry.mode | 0o755)
		var err error
		if entry.size != 0 {
			header.UncompressedSize64 = entry.size
			_, err = writer.CreateRaw(header)
		} else {
			var payload io.Writer
			payload, err = writer.CreateHeader(header)
			if err == nil {
				_, err = payload.Write(entry.data)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestExtractZIPStagesOnlyExactExecutable(t *testing.T) {
	const member = "ompig-slim/ompig-slim.exe"
	archive := zipFor(t, []zipEntry{
		{name: "ompig-slim/", mode: fs.ModeDir},
		{name: "ompig-slim/README.md", data: []byte("docs")},
		{name: member, data: []byte("released binary")},
	})
	dir := t.TempDir()
	staged, err := extractExecutable(writeTestArchive(t, archive), dir, releaseArtifact{member: member, format: zipArchive})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(staged) != dir || filepath.Ext(staged) != ".exe" {
		t.Fatalf("unexpected staging path %q", staged)
	}
	if got, err := os.ReadFile(staged); err != nil || string(got) != "released binary" {
		t.Fatalf("staged payload = %q, error = %v", got, err)
	}
	if files, err := os.ReadDir(dir); err != nil || len(files) != 1 {
		t.Fatalf("unexpected staged files %v, error = %v", files, err)
	}
}

func TestExtractZIPRejectsMalformedArchive(t *testing.T) {
	const member = "ompig-slim/ompig-slim.exe"
	valid := zipEntry{name: member, data: []byte("released binary")}
	for _, tc := range []struct {
		name    string
		entries []zipEntry
		corrupt bool
	}{
		{name: "missing"},
		{name: "wrong case", entries: []zipEntry{{name: "ompig-slim/OMPIG-SLIM.EXE", data: valid.data}}},
		{name: "directory", entries: []zipEntry{{name: member + "/", mode: fs.ModeDir}}},
		{name: "empty", entries: []zipEntry{{name: member}}},
		{name: "oversized", entries: []zipEntry{{name: member, size: maxArchiveBytes + 1}}},
		{name: "symlink", entries: []zipEntry{{name: member, data: []byte("other.exe"), mode: fs.ModeSymlink}}},
		{name: "unrelated symlink", entries: []zipEntry{valid, {name: "link", mode: fs.ModeSymlink}}},
		{name: "duplicate", entries: []zipEntry{valid, valid}},
		{name: "case alias", entries: []zipEntry{valid, {name: "ompig-slim/OMPIG-SLIM.EXE", data: valid.data}}},
		{name: "traversal", entries: []zipEntry{valid, {name: "ompig-slim/../other.exe"}}},
		{name: "backslash traversal", entries: []zipEntry{valid, {name: `ompig-slim\..\other.exe`}}},
		{name: "drive path", entries: []zipEntry{valid, {name: "C:/other.exe"}}},
		{name: "absolute path", entries: []zipEntry{valid, {name: "/other.exe"}}},
		{name: "bad CRC", entries: []zipEntry{valid}, corrupt: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := zipFor(t, tc.entries)
			if tc.corrupt {
				i := bytes.Index(archive, valid.data)
				if i < 0 {
					t.Fatal("stored ZIP payload not found")
				}
				archive[i] ^= 1
			}
			dir := t.TempDir()
			if _, err := extractExecutable(writeTestArchive(t, archive), dir, releaseArtifact{member: member, format: zipArchive}); err == nil {
				t.Fatal("malformed ZIP was accepted")
			}
			if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
				t.Fatalf("unexpected staged files %v, error = %v", files, err)
			}
		})
	}
}

func TestExtractTarGzipRejectsUnsafeEntries(t *testing.T) {
	const member = "ompig-slim/ompig-slim"
	for _, tc := range []struct {
		name   string
		header tar.Header
	}{
		{"duplicate", tar.Header{Name: member, Typeflag: tar.TypeReg}},
		{"traversal", tar.Header{Name: "ompig-slim/../other", Typeflag: tar.TypeReg}},
		{"symlink", tar.Header{Name: "ompig-slim/link", Typeflag: tar.TypeSymlink, Linkname: member}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			gzipWriter := gzip.NewWriter(&out)
			writer := tar.NewWriter(gzipWriter)
			if err := writer.WriteHeader(&tar.Header{Name: member, Typeflag: tar.TypeReg, Size: 3}); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Write([]byte("exe")); err != nil {
				t.Fatal(err)
			}
			if err := writer.WriteHeader(&tc.header); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gzipWriter.Close(); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if _, err := extractExecutable(writeTestArchive(t, out.Bytes()), dir, releaseArtifact{member: member, format: tarGzip}); err == nil {
				t.Fatal("malformed tarball was accepted")
			}
			if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
				t.Fatalf("unexpected staged files %v, error = %v", files, err)
			}
		})
	}
}

func writeTestArchive(t *testing.T, data []byte) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "release")
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}
