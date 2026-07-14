package dockerbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *dockerClient {
	c := newClient(srv.URL)
	c.http = srv.Client()
	return c
}

func TestPing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_ping" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if err := newTestClient(srv).ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestWaitReadyRetries(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := newTestClient(srv).waitReady(ctx, io.Discard); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
	if hits < 2 {
		t.Fatalf("expected retry, got %d hits", hits)
	}
}

func TestBuildImageStreamsAndDetectsError(t *testing.T) {
	// Success stream.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != imageTag {
			t.Errorf("missing tag: %v", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, `{"stream":"Step 1/2\n"}`+"\n"+`{"stream":"done\n"}`+"\n")
	}))
	defer srv.Close()
	var out bytes.Buffer
	if err := newTestClient(srv).buildImage(context.Background(), []byte("tar"), imageTag, &out); err != nil {
		t.Fatalf("buildImage: %v", err)
	}
	if !strings.Contains(out.String(), "Step 1/2") {
		t.Errorf("build output not streamed: %q", out.String())
	}

	// Error stream.
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"stream":"Step 1/2\n"}`+"\n"+`{"error":"boom"}`+"\n")
	}))
	defer errSrv.Close()
	err := newTestClient(errSrv).buildImage(context.Background(), []byte("tar"), imageTag, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected build error carrying daemon message, got %v", err)
	}
}

func TestContainerLifecycle(t *testing.T) {
	const wantID = "abc123"
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"Id":"`+wantID+`"}`)
	})
	mux.HandleFunc("/containers/"+wantID+"/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/containers/"+wantID+"/wait", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"StatusCode":0}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := newTestClient(srv)
	ctx := context.Background()

	id, err := c.createContainer(ctx, createOptions{Image: imageTag, Cmd: []string{"true"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id != wantID {
		t.Fatalf("id = %q, want %q", id, wantID)
	}
	if err := c.startContainer(ctx, id); err != nil {
		t.Fatalf("start: %v", err)
	}
	code, err := c.waitContainer(ctx, id)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestDemuxStream(t *testing.T) {
	// Build a framed stream: one stdout frame "hello", one stderr frame "err".
	var framed bytes.Buffer
	writeFrame := func(streamType byte, payload string) {
		var hdr [8]byte
		hdr[0] = streamType
		binary.BigEndian.PutUint32(hdr[4:], uint32(len(payload))) //nolint:gosec // test payloads are short literals
		framed.Write(hdr[:])
		framed.WriteString(payload)
	}
	writeFrame(1, "hello")
	writeFrame(2, "err")

	var out bytes.Buffer
	if err := demuxStream(&framed, &out); err != nil {
		t.Fatalf("demuxStream: %v", err)
	}
	if out.String() != "helloerr" {
		t.Fatalf("demuxed = %q, want %q", out.String(), "helloerr")
	}
}

func TestDemuxStreamUnframedFallback(t *testing.T) {
	// Data that does not start with a valid stream-type byte (0..2) is copied
	// through verbatim.
	raw := "plain text output from a TTY container"
	var out bytes.Buffer
	if err := demuxStream(strings.NewReader(raw), &out); err != nil {
		t.Fatalf("demuxStream: %v", err)
	}
	if out.String() != raw {
		t.Fatalf("fallback copy = %q, want %q", out.String(), raw)
	}
}

func TestImageExists(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{http.StatusOK, true},
		{http.StatusNotFound, false},
	}
	for _, tc := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/images/") {
				t.Errorf("unexpected path %q", r.URL.Path)
			}
			w.WriteHeader(tc.status)
			if tc.status == http.StatusOK {
				_, _ = io.WriteString(w, `{"Id":"sha256:x"}`)
			}
		}))
		got, err := newTestClient(srv).imageExists(context.Background(), imageTag)
		srv.Close()
		if err != nil {
			t.Fatalf("imageExists: %v", err)
		}
		if got != tc.want {
			t.Fatalf("imageExists(status %d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestRemoveContainerIgnoresNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if err := newTestClient(srv).removeContainer(context.Background(), "gone"); err != nil {
		t.Fatalf("removeContainer should ignore 404, got %v", err)
	}
}
