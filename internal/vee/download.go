package vee

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Version is the vee release jk2coop downloads. Pinned (rather than "latest") so
// a jk2coop build always fetches a vee it was tested against and the download is
// reproducible. Bump it deliberately when adopting a newer vee.
const Version = "v0.1.0"

// releaseBase is the GitHub releases download URL prefix. Indirected as a var so
// tests can point it at a local server.
var releaseBase = "https://github.com/Benehiko/vee/releases/download"

// httpClient is indirected for testing.
var httpClient = http.DefaultClient

// assetName is the release tarball for the running OS/arch, e.g.
// vee-v0.1.0-linux-amd64.tar.gz. vee publishes darwin/linux/windows on
// amd64/arm64.
func assetName() (string, error) {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	switch goos {
	case "linux", "darwin", "windows":
	default:
		return "", fmt.Errorf("no prebuilt vee for GOOS=%s; install vee manually (https://github.com/Benehiko/vee)", goos)
	}
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("no prebuilt vee for GOARCH=%s; install vee manually (https://github.com/Benehiko/vee)", goarch)
	}
	return fmt.Sprintf("vee-%s-%s-%s.tar.gz", Version, goos, goarch), nil
}

// download fetches the pinned vee release for this platform, verifies its
// published SHA-256, extracts the vee binary into the managed config-dir bin,
// and returns its path.
func download(ctx context.Context, out interface{ Write([]byte) (int, error) }) (string, error) {
	asset, err := assetName()
	if err != nil {
		return "", err
	}
	dir, err := ManagedDir()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(dir, binName())

	url := releaseBase + "/" + Version + "/" + asset
	_, _ = fmt.Fprintf(out, "Downloading vee %s (%s)…\n", Version, url)

	tarball, err := httpGet(ctx, url)
	if err != nil {
		return "", fmt.Errorf("downloading vee: %w", err)
	}

	want, err := fetchSHA256(ctx, url+".sha256")
	if err != nil {
		return "", fmt.Errorf("fetching vee checksum: %w", err)
	}
	got := sha256.Sum256(tarball)
	if hex.EncodeToString(got[:]) != want {
		return "", fmt.Errorf("vee checksum mismatch: got %x, want %s (refusing to install)", got, want)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := extractBinary(tarball, dest); err != nil {
		return "", fmt.Errorf("extracting vee: %w", err)
	}
	_, _ = fmt.Fprintf(out, "Installed vee to %s\n", dest)
	return dest, nil
}

// httpGet fetches url and returns the full body.
func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// fetchSHA256 downloads a `<hash>  <filename>` checksum file and returns the
// lowercase hex hash from its first line.
func fetchSHA256(ctx context.Context, url string) (string, error) {
	b, err := httpGet(ctx, url)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return "", fmt.Errorf("empty checksum file %s", url)
	}
	return strings.ToLower(fields[0]), nil
}

// extractBinary pulls the `vee` (or `vee.exe`) entry out of the gzip'd tar and
// writes it to dest with the executable bit set.
func extractBinary(tarball []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(tarball))
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	want := binName()
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%q not found in vee archive", want)
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) != want {
			continue
		}
		return writeExecutable(dest, tr)
	}
}

// writeExecutable writes r to dest atomically-ish (temp file + rename) with mode
// 0o755, so a partial download never leaves a half-written binary at dest.
func writeExecutable(dest string, r io.Reader) error {
	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}
