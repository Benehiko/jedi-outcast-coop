package dockerbuild

import (
	"archive/tar"
	"bytes"
	_ "embed"
	"fmt"
)

// dockerfile is the build image definition, tarred as the /build context.
//
//go:embed Dockerfile
var dockerfile []byte

// buildContext returns a tar archive containing just the Dockerfile, suitable
// as the POST /build request body. The build needs no other context files — the
// source is bind-mounted at run time, not copied into the image.
func buildContext() ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name: "Dockerfile",
		Mode: 0o644,
		Size: int64(len(dockerfile)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, fmt.Errorf("writing Dockerfile tar header: %w", err)
	}
	if _, err := tw.Write(dockerfile); err != nil {
		return nil, fmt.Errorf("writing Dockerfile to tar: %w", err)
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing build-context tar: %w", err)
	}
	return buf.Bytes(), nil
}
