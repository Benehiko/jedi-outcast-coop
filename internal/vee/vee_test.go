package vee

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolvePrefersPath(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(string) (string, error) { return "/usr/bin/vee", nil }

	got, ok := Resolve()
	if !ok || got != "/usr/bin/vee" {
		t.Fatalf("Resolve() = %q, %v; want /usr/bin/vee, true", got, ok)
	}
}

func TestResolveFallsBackToManaged(t *testing.T) {
	dir := t.TempDir()
	stubLookPath(t, errors.New("not found"))
	stubConfigDir(t, dir)

	// No managed binary yet → not found.
	if _, ok := Resolve(); ok {
		t.Fatal("Resolve() found a vee with none installed")
	}

	// Create the managed binary → found.
	binDir := filepath.Join(dir, "jk2coop", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(binDir, binName())
	if err := os.WriteFile(managed, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := Resolve()
	if !ok || got != managed {
		t.Fatalf("Resolve() = %q, %v; want %q, true", got, ok, managed)
	}
}

func TestEnsureDownloadsAndVerifies(t *testing.T) {
	dir := t.TempDir()
	stubLookPath(t, errors.New("not found"))
	stubConfigDir(t, dir)

	tarball := makeVeeTarball(t, "vee-binary-payload")
	sum := sha256.Sum256(tarball)
	asset, err := assetName()
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, asset+".sha256"):
			_, _ = fmt.Fprintf(w, "%x  %s\n", sum, asset)
		case strings.HasSuffix(r.URL.Path, asset):
			_, _ = w.Write(tarball)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	origBase := releaseBase
	origClient := httpClient
	t.Cleanup(func() { releaseBase, httpClient = origBase, origClient })
	releaseBase = srv.URL
	httpClient = srv.Client()

	path, err := Ensure(context.Background(), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Ensure() error: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "vee-binary-payload" {
		t.Fatalf("extracted binary = %q; want payload", b)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode()&0o111 == 0 {
		t.Errorf("extracted binary not executable: %v", fi.Mode())
	}
}

func TestEnsureRejectsBadChecksum(t *testing.T) {
	dir := t.TempDir()
	stubLookPath(t, errors.New("not found"))
	stubConfigDir(t, dir)

	tarball := makeVeeTarball(t, "payload")
	asset, _ := assetName()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			_, _ = fmt.Fprintf(w, "%s  %s\n", strings.Repeat("0", 64), asset)
			return
		}
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	origBase, origClient := releaseBase, httpClient
	t.Cleanup(func() { releaseBase, httpClient = origBase, origClient })
	releaseBase, httpClient = srv.URL, srv.Client()

	if _, err := Ensure(context.Background(), &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Ensure() error = %v; want checksum mismatch", err)
	}
	// A bad download must leave no binary behind.
	managed, _ := ManagedPath()
	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Errorf("managed binary exists after failed verify: %v", err)
	}
}

func stubLookPath(t *testing.T, err error) {
	t.Helper()
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(string) (string, error) { return "", err }
}

func stubConfigDir(t *testing.T, dir string) {
	t.Helper()
	orig := userConfigDir
	t.Cleanup(func() { userConfigDir = orig })
	userConfigDir = func() (string, error) { return dir, nil }
}

// makeVeeTarball builds a gzip'd tar containing a single vee binary entry with
// the given payload, plus a decoy file, mirroring the real release layout.
func makeVeeTarball(t *testing.T, payload string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	entries := []struct {
		name string
		body string
	}{
		{"LICENSE", "license text"},
		{binName(), payload},
	}
	for _, e := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
