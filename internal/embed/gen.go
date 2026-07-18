//go:build ignore

// Command gen regenerates the embedded assets for internal/embed:
//
//   - openjk-src.tar.gz — the pruned OpenJK source tree (only the dirs the
//     JK2-SP build compiles, plus codemp so the full patch set applies), taken
//     from the pinned submodule at ../../openjk.
//   - pin.txt — the submodule commit the archive was built from, used at runtime
//     to detect a stale extracted tree and to stamp builds.
//
// Run it via `go generate ./internal/embed` after bumping the submodule. It
// reads the submodule through `git archive`, so it captures exactly the tracked
// files at HEAD (no untracked/build artifacts), and it fails loudly if the
// submodule is missing or dirty.
//
// The patch files and coop-ui assets are embedded directly from patches/ and
// assets/ via go:embed (see embed.go); only the large source tree needs the
// archive indirection.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// keep is the set of top-level paths under the submodule to include. Verified
// against the JK2-SP CMake target inputs: these are every dir the build
// compiles or #includes, plus codemp (dead to the SP build but kept so the MP
// patches 0001-0003 apply to a real tree). Everything else (tests, tools,
// scripts, docs, top-level ui, bundled jpeg/zlib/png/SDL2, .git, build) is
// pruned — see docs/embedded-source.md.
var keep = []string{
	"CMakeLists.txt",
	// LICENSE.txt + README.md are required at CONFIGURE time: InstallConfig.cmake
	// hands them to CPack as the license/readme resources, which errors out if
	// either is absent — even though we never build a package.
	"LICENSE.txt",
	"README.md",
	"cmake",
	"code",
	"codeJK2",
	"codemp",
	"shared",
	"lib/minizip",
	"lib/gsl-lite",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run() error {
	here, err := os.Getwd() // internal/embed when run via go generate
	if err != nil {
		return err
	}
	sub := filepath.Join(here, "..", "..", "openjk")
	if _, err := os.Stat(filepath.Join(sub, ".git")); err != nil {
		return fmt.Errorf("submodule not checked out at %s: run: git submodule update --init", sub)
	}

	pin, err := gitOut(sub, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("reading submodule pin: %w", err)
	}
	pin = strings.TrimSpace(pin)

	// Warn (don't fail) on a dirty submodule: the archive is built from
	// `git archive HEAD`, which reads the committed tree at the pin and ignores
	// working-tree edits, so applied patches / untracked files do not leak in.
	if status, err := gitOut(sub, "status", "--porcelain"); err != nil {
		return err
	} else if strings.TrimSpace(status) != "" {
		fmt.Fprintf(os.Stderr, "gen: note: submodule at %s has working-tree changes; "+
			"archiving the committed tree at %s (dirt ignored)\n", sub, pin[:12])
	}

	archive, err := buildArchive(sub)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(here, "openjk-src.tar.gz"), archive, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(here, "pin.txt"), []byte(pin+"\n"), 0o644); err != nil {
		return err
	}

	// Mirror patches/ and assets/coop-ui/ into this package so a single
	// go:embed (embed.go) can reach them — go:embed cannot cross up out of the
	// package directory. These are the source of truth for what the binary ships.
	root := filepath.Join(here, "..", "..")
	if err := mirrorDir(filepath.Join(root, "patches"), filepath.Join(here, "patches"), ".patch", ""); err != nil {
		return fmt.Errorf("mirroring patches: %w", err)
	}
	// Mirror only the coop-ui SOURCE (the ui/ tree). The built zz-coop-ui.pk3 is
	// a gitignored artifact — absent on a fresh CI checkout — so embedding it
	// would make the generated tree machine-dependent. The installer rebuilds
	// the pak from the embedded ui/ tree (paks.BuildCoopUI), so the source is
	// all we need to ship.
	if err := mirrorDir(filepath.Join(root, "assets", "coop-ui"), filepath.Join(here, "coop-ui"), "", ".pk3"); err != nil {
		return fmt.Errorf("mirroring coop-ui: %w", err)
	}
	// Mirror the blaster-fx SOURCE (the effects/ tree) for the same reason: the
	// built zz-blaster-fx.pk3 is a gitignored artifact; the installer rebuilds it
	// from the embedded effects/ tree (paks.BuildBlasterFX).
	if err := mirrorDir(filepath.Join(root, "assets", "blaster-fx"), filepath.Join(here, "blaster-fx"), "", ".pk3"); err != nil {
		return fmt.Errorf("mirroring blaster-fx: %w", err)
	}

	fmt.Printf("wrote openjk-src.tar.gz (%d bytes) + pin.txt (%s) + patches/ + coop-ui/ + blaster-fx/\n", len(archive), pin[:12])
	return nil
}

// mirrorDir copies every file under src (recursively) into dst, replacing dst.
// When onlyExt is non-empty, only files with that extension are copied (used for
// patches/*.patch). When skipExt is non-empty, files with that extension are
// excluded (used to drop the gitignored built pak from coop-ui).
func mirrorDir(src, dst, onlyExt, skipExt string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if onlyExt != "" && filepath.Ext(p) != onlyExt {
			return nil
		}
		if skipExt != "" && filepath.Ext(p) == skipExt {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
}

// buildArchive tars the keep-set from the submodule (via `git archive` to honour
// tracked-only + .gitattributes) and gzips it. It re-tars the git-archive output
// filtered to the keep-set, so paths are relative to the tree root.
func buildArchive(sub string) ([]byte, error) {
	// `git archive HEAD -- <keep...>` emits a tar of only the requested paths.
	args := append([]string{"-C", sub, "archive", "--format=tar", "HEAD", "--"}, keep...)
	raw, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git archive: %w", err)
	}

	// Re-pack deterministically (sorted entries, zeroed mtime/uid/gid) so the
	// committed artifact is stable and only changes when the source does. The
	// gzip stream is produced by the Go toolchain's compress/gzip, so byte-for-
	// byte reproducibility holds for a fixed Go version — CI pins the toolchain
	// via go-version-file, so `make verify-embed` compares like with like.
	entries, err := readTar(raw)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		hdr := &tar.Header{
			Name: e.name,
			Mode: 0o644,
			Size: int64(len(e.data)),
		}
		if e.dir {
			hdr.Typeflag = tar.TypeDir
			hdr.Mode = 0o755
			hdr.Name = strings.TrimSuffix(e.name, "/") + "/"
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if !e.dir {
			if _, err := tw.Write(e.data); err != nil {
				return nil, err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type entry struct {
	name string
	data []byte
	dir  bool
}

func readTar(raw []byte) ([]entry, error) {
	tr := tar.NewReader(bytes.NewReader(raw))
	var out []entry
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		name := path.Clean(hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			out = append(out, entry{name: name, dir: true})
		case tar.TypeReg:
			data := make([]byte, hdr.Size)
			if _, err := readFull(tr, data); err != nil {
				return nil, err
			}
			out = append(out, entry{name: name, data: data})
		}
	}
	return out, nil
}

func readFull(tr *tar.Reader, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		m, err := tr.Read(b[n:])
		n += m
		if err != nil {
			if n == len(b) {
				return n, nil
			}
			return n, err
		}
	}
	return n, nil
}

func gitOut(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	return string(out), err
}
