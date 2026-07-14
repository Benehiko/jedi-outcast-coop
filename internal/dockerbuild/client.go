package dockerbuild

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// dockerClient is a minimal Docker Engine API client speaking plain HTTP to the
// daemon inside the vee "docker" template VM (forwarded to tcp://127.0.0.1:2375
// on the host). It implements only the handful of endpoints the build needs —
// there is deliberately no dependency on the Docker SDK or testcontainers.
type dockerClient struct {
	base string // e.g. "http://127.0.0.1:2375"
	http *http.Client
}

// newClient returns a client for the daemon at base (no trailing slash).
func newClient(base string) *dockerClient {
	return &dockerClient{
		base: strings.TrimRight(base, "/"),
		// No timeout on the client itself: image build and container runs are
		// long, and each call passes a context for cancellation/deadlines.
		http: &http.Client{},
	}
}

// ping reports whether the daemon answers GET /_ping. Used to wait for dockerd
// to finish starting inside a freshly booted VM.
func (c *dockerClient) ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/_ping", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker ping: HTTP %d", resp.StatusCode)
	}
	return nil
}

// waitReady polls ping until it succeeds or ctx is done. It is tolerant of the
// daemon not yet listening (connection refused) during VM boot.
func (c *dockerClient) waitReady(ctx context.Context, out io.Writer) error {
	const interval = 2 * time.Second
	for {
		if err := c.ping(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("docker daemon not ready: %w", ctx.Err())
		case <-time.After(interval):
			_, _ = fmt.Fprint(out, ".")
		}
	}
}

// buildImage builds an image from a tar build context (containing at least a
// Dockerfile named "Dockerfile"), tagging it tag. Build output (the daemon's
// JSON stream) is decoded and the human-readable lines are written to out. A
// build step failure is reported as an error carrying the daemon's message.
func (c *dockerClient) buildImage(ctx context.Context, tarContext []byte, tag string, out io.Writer) error {
	q := url.Values{"t": {tag}, "rm": {"1"}, "forcerm": {"1"}}
	u := c.base + "/build?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(tarContext))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("image build: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return streamBuildJSON(resp.Body, out)
}

// buildMessage is one line of the /build (and /images/create) JSON stream.
type buildMessage struct {
	Stream string `json:"stream"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

// streamBuildJSON decodes the daemon's newline-delimited JSON build stream,
// forwarding stream/status text to out and returning the first error line.
func streamBuildJSON(r io.Reader, out io.Writer) error {
	dec := json.NewDecoder(r)
	for {
		var m buildMessage
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decoding build stream: %w", err)
		}
		if m.Error != "" {
			return fmt.Errorf("build failed: %s", m.Error)
		}
		if m.Stream != "" {
			_, _ = io.WriteString(out, m.Stream)
		} else if m.Status != "" {
			_, _ = fmt.Fprintln(out, m.Status)
		}
	}
}

// createOptions are the container fields the build sets.
type createOptions struct {
	Image      string
	Cmd        []string
	Binds      []string // "hostPath:containerPath[:ro]"; hostPath is inside the VM
	WorkingDir string
}

// createContainer creates a container and returns its ID.
func (c *dockerClient) createContainer(ctx context.Context, opts createOptions) (string, error) {
	body := map[string]any{
		"Image":      opts.Image,
		"Cmd":        opts.Cmd,
		"WorkingDir": opts.WorkingDir,
		"HostConfig": map[string]any{"Binds": opts.Binds},
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/containers/create", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("container create: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var created struct {
		ID string `json:"Id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decoding create response: %w", err)
	}
	return created.ID, nil
}

// startContainer starts a created container.
func (c *dockerClient) startContainer(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/containers/"+id+"/start", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("container start: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// waitContainer blocks until the container exits and returns its exit code.
func (c *dockerClient) waitContainer(ctx context.Context, id string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/containers/"+id+"/wait", nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("container wait: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var res struct {
		StatusCode int `json:"StatusCode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, fmt.Errorf("decoding wait response: %w", err)
	}
	return res.StatusCode, nil
}

// streamLogs streams a container's stdout+stderr to out, following until the
// container exits. The Docker log stream is multiplexed with an 8-byte header
// per frame when the container has no TTY; demuxStream unwraps it.
func (c *dockerClient) streamLogs(ctx context.Context, id string, out io.Writer) error {
	q := url.Values{"stdout": {"1"}, "stderr": {"1"}, "follow": {"1"}}
	u := c.base + "/containers/" + id + "/logs?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("container logs: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return demuxStream(resp.Body, out)
}

// demuxStream unwraps Docker's multiplexed log framing (8-byte header:
// [stream_type, 0,0,0, size_be_uint32] then size payload bytes) and writes the
// payloads to out. If the data does not look framed (e.g. a TTY container), it
// falls back to copying verbatim.
func demuxStream(r io.Reader, out io.Writer) error {
	var header [8]byte
	for {
		if _, err := io.ReadFull(r, header[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}
		// Valid stream types are 0 (stdin), 1 (stdout), 2 (stderr). Anything
		// else means the data is not framed — write what we read and copy the
		// rest through unmodified.
		if header[0] > 2 {
			if _, err := out.Write(header[:]); err != nil {
				return err
			}
			_, err := io.Copy(out, r)
			return err
		}
		size := binary.BigEndian.Uint32(header[4:])
		if _, err := io.CopyN(out, r, int64(size)); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// removeContainer deletes a container (force + volumes), ignoring "not found".
func (c *dockerClient) removeContainer(ctx context.Context, id string) error {
	q := url.Values{"force": {"1"}, "v": {"1"}}
	u := c.base + "/containers/" + id + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("container remove: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// imageExists reports whether an image with the given tag is present, so a warm
// VM can skip the (slow) image build on a re-run.
func (c *dockerClient) imageExists(ctx context.Context, tag string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/images/"+url.PathEscape(tag)+"/json", nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		b, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("image inspect: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
}

// apiPort is the Docker API port the vee docker template forwards to the host.
const apiPort = 2375

// defaultBase is the daemon endpoint on the host (vee user-mode port forward).
func defaultBase() string {
	return "http://127.0.0.1:" + strconv.Itoa(apiPort)
}
