package textures

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// upscaleStampName is the archive member BuildUpscaledPak writes into the output
// pak to record the fingerprint of the inputs that produced it. On a re-run with
// an unchanged fingerprint the whole (slow) pipeline is skipped. The dot-prefix
// keeps it out of the engine's texture namespace.
const upscaleStampName = ".jk2coop-upscale-stamp"

// upscaleFingerprint derives a stable fingerprint of everything that affects the
// upscaled output: the pipeline knobs and the identity (path, size, mtime) of
// each source retail pak. If any of these change, the fingerprint changes and
// the pak is rebuilt.
func upscaleFingerprint(paks []string, opts UpscaleOptions) (string, error) {
	h := sha256.New()
	// hash.Hash.Write never returns an error, so the Fprintf writes below cannot
	// fail; the returns are ignored deliberately.
	//
	// Version tag: bump when the pipeline's output changes for identical inputs
	// (e.g. an encoder tweak), to force a rebuild for existing users.
	_, _ = fmt.Fprintf(h, "v1\n")
	_, _ = fmt.Fprintf(h, "scale=%d\nmaxsize=%d\nmodel=%s\nimage=%s\nstub=%t\nlimit=%d\n",
		opts.Scale, opts.MaxSize, opts.Model, opts.Image, opts.Stub, opts.Limit)

	sorted := append([]string(nil), paks...)
	sort.Strings(sorted)
	for _, p := range sorted {
		fi, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		// Base name (not full path) so relocating the game dir doesn't force a
		// needless rebuild; size + mtime catch a changed/replaced pak.
		_, _ = fmt.Fprintf(h, "pak=%s size=%d mtime=%d\n",
			filepath.Base(p), fi.Size(), fi.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// readUpscaleStamp returns the fingerprint recorded in an existing output pak,
// or "" if the pak is absent, unreadable, or carries no stamp.
func readUpscaleStamp(pakPath string) string {
	r, err := pk3.Open(pakPath)
	if err != nil {
		return ""
	}
	defer func() { _ = r.Close() }()
	data, err := r.ReadFile(upscaleStampName)
	if err != nil {
		return ""
	}
	return string(data)
}
