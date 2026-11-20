package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

const archiveMember = "ompig-slim/ompig-slim"

type archiveFormat uint8

const (
	tarGzip archiveFormat = iota
	zipArchive
)

type releaseArtifact struct {
	name   string
	member string
	format archiveFormat
}

func artifactFor(goos, goarch string) (releaseArtifact, error) {
	var arch string
	switch goarch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return releaseArtifact{}, fmt.Errorf("no release archive for CPU %s (supported: x86_64, arm64)", goarch)
	}
	base := "ompig-slim-" + goos + "-" + arch
	switch goos {
	case "linux", "darwin":
		return releaseArtifact{base + ".tar.gz", archiveMember, tarGzip}, nil
	case "windows":
		return releaseArtifact{base + ".zip", archiveMember + ".exe", zipArchive}, nil
	default:
		return releaseArtifact{}, fmt.Errorf("no release archive for %s (supported: Linux, macOS, Windows)", goos)
	}
}

func extractExecutable(archive, dir string, artifact releaseArtifact) (string, error) {
	if artifact.format == zipArchive {
		return extractZIP(archive, dir, artifact.member)
	}
	return extractTarGzip(archive, dir, artifact.member)
}

func extractZIP(archive, dir, member string) (string, error) {
	reader, err := zip.OpenReader(archive)
	if reader != nil {
		defer reader.Close()
	}
	if err != nil {
		return "", fmt.Errorf("read release ZIP: %w", err)
	}
	var executable *zip.File
	seen := make(map[string]bool, len(reader.File))
	for _, entry := range reader.File {
		mode := entry.Mode()
		name, err := archiveEntryName(entry.Name, mode)
		if err != nil {
			return "", err
		}
		key := strings.ToUpper(name)
		if seen[key] {
			return "", fmt.Errorf("duplicate archive entry %q", name)
		}
		seen[key] = true
		if entry.Name == member {
			executable = entry
		}
	}
	if executable == nil {
		return "", fmt.Errorf("release archive does not contain %s", member)
	}
	if !executable.Mode().IsRegular() || executable.UncompressedSize64 == 0 || executable.UncompressedSize64 > maxArchiveBytes {
		return "", fmt.Errorf("%s must be a nonempty regular file of at most %d bytes", member, maxArchiveBytes)
	}
	payload, err := executable.Open()
	if err != nil {
		return "", fmt.Errorf("open %s: %w", member, err)
	}
	defer payload.Close()
	return stageExecutable(dir, member, payload, int64(executable.UncompressedSize64))
}

func extractTarGzip(archive, dir, member string) (_ string, err error) {
	file, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer file.Close()
	uncompressed, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("read release archive: %w", err)
	}
	defer uncompressed.Close()

	body := &io.LimitedReader{R: uncompressed, N: 2 * maxArchiveBytes}
	reader := tar.NewReader(body)
	seen := map[string]bool{}
	var staged string
	defer func() {
		if err != nil && staged != "" {
			_ = os.Remove(staged)
		}
	}()
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read release archive: %w", err)
		}
		name, err := archiveEntryName(header.Name, header.FileInfo().Mode())
		if err != nil {
			return "", err
		}
		if seen[name] {
			return "", fmt.Errorf("duplicate archive entry %q", name)
		}
		seen[name] = true
		if header.Name != member {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maxArchiveBytes {
			return "", fmt.Errorf("%s must be a nonempty regular file of at most %d bytes", member, maxArchiveBytes)
		}
		staged, err = stageExecutable(dir, member, reader, header.Size)
		if err != nil {
			return "", err
		}
	}
	if _, err := io.Copy(io.Discard, body); err != nil {
		return "", fmt.Errorf("read release archive: %w", err)
	}
	if body.N == 0 {
		return "", errors.New("uncompressed release archive exceeds the size limit")
	}
	if staged == "" {
		return "", fmt.Errorf("release archive does not contain %s", member)
	}
	return staged, nil
}

func archiveEntryName(name string, mode fs.FileMode) (string, error) {
	if !mode.IsRegular() && !mode.IsDir() {
		return "", fmt.Errorf("archive entry %q is not a regular file or directory", name)
	}
	if mode.IsDir() {
		name = strings.TrimSuffix(name, "/")
	}
	if name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:\x00") {
		return "", fmt.Errorf("unsafe archive entry %q", name)
	}
	return name, nil
}

func stageExecutable(dir, member string, payload io.Reader, size int64) (string, error) {
	file, err := os.CreateTemp(dir, ".ompig-slim-update-*"+path.Ext(member))
	if err != nil {
		return "", fmt.Errorf("stage update (is %s writable?): %w", dir, err)
	}
	name := file.Name()
	written, copyErr := io.Copy(file, io.LimitReader(payload, size+1))
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("copy %s: %w", member, err)
	}
	if written != size {
		_ = os.Remove(name)
		return "", fmt.Errorf("copy %s: wrote %d of %d bytes", member, written, size)
	}
	if err := os.Chmod(name, 0o755); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}
