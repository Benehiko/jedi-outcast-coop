package textures

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/Benehiko/jedi-outcast-coop/internal/pk3"
)

// makeImage builds a small w×h test image with a deterministic per-pixel colour
// so codec round-trips can be checked structurally.
func makeImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8((x * 7) % 256),
				G: uint8((y * 11) % 256),
				B: uint8((x + y) % 256),
				A: 0xFF,
			})
		}
	}
	return img
}

func TestTGARoundTrip(t *testing.T) {
	src := makeImage(6, 4)
	var buf bytes.Buffer
	if err := EncodeTGA(&buf, src); err != nil {
		t.Fatalf("EncodeTGA: %v", err)
	}
	got, err := DecodeTGA(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("DecodeTGA: %v", err)
	}
	if got.Bounds() != src.Bounds() {
		t.Fatalf("bounds = %v, want %v", got.Bounds(), src.Bounds())
	}
	for y := range 4 {
		for x := range 6 {
			want := src.NRGBAAt(x, y)
			c := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
			if c != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, c, want)
			}
		}
	}
}

func TestDecodeImageDispatch(t *testing.T) {
	src := makeImage(4, 4)

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, src); err != nil {
		t.Fatal(err)
	}
	var jpgBuf bytes.Buffer
	if err := jpeg.Encode(&jpgBuf, src, nil); err != nil {
		t.Fatal(err)
	}
	var tgaBuf bytes.Buffer
	if err := EncodeTGA(&tgaBuf, src); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"a.png", pngBuf.Bytes()},
		{"a.jpg", jpgBuf.Bytes()},
		{"a.tga", tgaBuf.Bytes()},
	} {
		img, err := DecodeImage(tc.name, tc.data)
		if err != nil {
			t.Fatalf("DecodeImage(%s): %v", tc.name, err)
		}
		if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 4 {
			t.Fatalf("%s: bounds = %v", tc.name, img.Bounds())
		}
	}

	if _, err := DecodeImage("bad.bin", []byte("not an image")); err == nil {
		t.Fatal("DecodeImage on garbage: want error, got nil")
	}
}

func TestSnapToPowerOfTwo(t *testing.T) {
	for _, tc := range []struct {
		w, h, wantW, wantH int
	}{
		{96, 128, 128, 128}, // non-PoT width snaps up
		{64, 64, 64, 64},    // already PoT: unchanged
		{100, 200, 128, 256},
		{1, 3, 1, 4},
	} {
		out := SnapToPowerOfTwo(makeImage(tc.w, tc.h))
		if out.Bounds().Dx() != tc.wantW || out.Bounds().Dy() != tc.wantH {
			t.Fatalf("snap %dx%d = %dx%d, want %dx%d",
				tc.w, tc.h, out.Bounds().Dx(), out.Bounds().Dy(), tc.wantW, tc.wantH)
		}
	}
}

func TestFormatForExt(t *testing.T) {
	for _, tc := range []struct {
		ext  string
		want Format
	}{
		{"tga", FormatTGA},
		{".TGA", FormatTGA},
		{"jpg", FormatJPEG},
		{"jpeg", FormatJPEG},
		{"png", FormatPNG},
		{"unknown", FormatPNG},
	} {
		if got := FormatForExt(tc.ext); got != tc.want {
			t.Fatalf("FormatForExt(%q) = %v, want %v", tc.ext, got, tc.want)
		}
	}
}

func TestIsRasterTexture(t *testing.T) {
	yes := []string{
		"textures/wall/metal.jpg",
		"models/players/head.tga",
		"TEXTURES/Foo/Bar.PNG",
		"textures/x.jpeg",
	}
	no := []string{
		"gfx/2d/hud.jpg",      // 2D HUD not under textures/ or models/
		"fonts/font.tga",      // fonts skipped
		"textures/readme.txt", // not a raster
		"scripts/x.shader",
	}
	for _, n := range yes {
		if !isRasterTexture(n) {
			t.Fatalf("isRasterTexture(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if isRasterTexture(n) {
			t.Fatalf("isRasterTexture(%q) = true, want false", n)
		}
	}
}

func TestParseManifest(t *testing.T) {
	content := "# a comment\n\nmetal|a metal prompt\n rock | a rock prompt \nbroken line no pipe\n"
	m, err := ParseManifest(content)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(m), m)
	}
	if m[0].Name != "metal" || m[0].Prompt != "a metal prompt" {
		t.Fatalf("entry 0 = %+v", m[0])
	}
	if m[1].Name != "rock" || m[1].Prompt != "a rock prompt" {
		t.Fatalf("entry 1 = %+v", m[1])
	}

	if _, err := ParseManifest("# only comments\n\n"); err == nil {
		t.Fatal("ParseManifest with no entries: want error")
	}
}

// makePak writes a pk3 with the given archivePath -> image entries, encoding
// each by the path's extension.
func makePak(t *testing.T, path string, entries map[string]*image.NRGBA) {
	t.Helper()
	dir := t.TempDir()
	b := pk3.NewBuilder()
	for arc, img := range entries {
		src := filepath.Join(dir, filepath.FromSlash(arc))
		if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := EncodeFile(src, img, FormatForExt(filepath.Ext(arc))); err != nil {
			t.Fatal(err)
		}
		b.Add(arc, src)
	}
	if err := b.Write(path); err != nil {
		t.Fatalf("write pak: %v", err)
	}
}

func TestIndexTexturesLastPakWins(t *testing.T) {
	dir := t.TempDir()
	pak0 := filepath.Join(dir, "assets0.pk3")
	pak1 := filepath.Join(dir, "assets1.pk3")
	makePak(t, pak0, map[string]*image.NRGBA{
		"textures/a.jpg": makeImage(8, 8),
		"textures/b.tga": makeImage(8, 8),
		"gfx/hud.jpg":    makeImage(8, 8), // not a texture; must be skipped
	})
	makePak(t, pak1, map[string]*image.NRGBA{
		"textures/a.jpg": makeImage(16, 16), // overrides pak0
		"models/c.png":   makeImage(8, 8),
	})

	entries, err := indexTextures([]string{pak0, pak1})
	if err != nil {
		t.Fatalf("indexTextures: %v", err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.name] = filepath.Base(e.pak)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3: %v", len(got), got)
	}
	if got["textures/a.jpg"] != "assets1.pk3" {
		t.Fatalf("textures/a.jpg should resolve from assets1.pk3 (last wins), got %q", got["textures/a.jpg"])
	}
	if _, ok := got["gfx/hud.jpg"]; ok {
		t.Fatal("gfx/hud.jpg should not be indexed")
	}
}

func TestBuildUpscaledPakStub(t *testing.T) {
	dir := t.TempDir()
	assets := filepath.Join(dir, "base")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	// A non-power-of-two source so the PoT snap must act.
	makePak(t, filepath.Join(assets, "assets0.pk3"), map[string]*image.NRGBA{
		"textures/wall.jpg": makeImage(96, 96),
		"models/skin.tga":   makeImage(64, 64),
	})

	out := filepath.Join(dir, "zzz-hires-textures.pk3")
	res, err := BuildUpscaledPak(context.Background(), UpscaleOptions{
		AssetsDir: assets,
		OutPath:   out,
		Scale:     2,
		Stub:      true,
	})
	if err != nil {
		t.Fatalf("BuildUpscaledPak: %v", err)
	}
	if res.Textures != 2 {
		t.Fatalf("packed %d textures, want 2", res.Textures)
	}

	r, err := pk3.Open(out)
	if err != nil {
		t.Fatalf("open output pak: %v", err)
	}
	defer func() { _ = r.Close() }()

	// The wall was 96 -> stub 2x = 192 -> PoT snap = 256; extension preserved.
	if !r.Has("textures/wall.jpg") {
		t.Fatalf("output pak missing textures/wall.jpg; has %v", r.Names())
	}
	if !r.Has("models/skin.tga") {
		t.Fatalf("output pak missing models/skin.tga; has %v", r.Names())
	}
	data, err := r.ReadFile("textures/wall.jpg")
	if err != nil {
		t.Fatal(err)
	}
	img, err := DecodeImage("wall.jpg", data)
	if err != nil {
		t.Fatalf("decode packed wall: %v", err)
	}
	if img.Bounds().Dx() != 256 || img.Bounds().Dy() != 256 {
		t.Fatalf("packed wall = %v, want 256x256", img.Bounds())
	}
}
