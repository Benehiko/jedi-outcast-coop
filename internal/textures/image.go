package textures

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// Format is a supported texture image format on disk.
type Format int

const (
	// FormatPNG is a lossless PNG. Used for the model-input normalisation.
	FormatPNG Format = iota
	// FormatJPEG is a quality-95 JPEG, used for diffuse colour maps.
	FormatJPEG
	// FormatTGA is uncompressed 32-bit truecolour Targa.
	FormatTGA
)

// jpegQuality matches the shell tools' `-quality 95`.
const jpegQuality = 95

// FormatForExt maps a file extension (with or without leading dot,
// case-insensitive) to the Format used to re-encode at that path. Unknown
// extensions fall back to PNG.
func FormatForExt(ext string) Format {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "tga":
		return FormatTGA
	case "jpg", "jpeg":
		return FormatJPEG
	default:
		return FormatPNG
	}
}

// DecodeImage decodes a texture from raw bytes, dispatching on content. The
// standard library handles JPEG and PNG; TGA is handled here. The name is used
// only to give a better error message.
func DecodeImage(name string, data []byte) (image.Image, error) {
	// image.Decode auto-detects PNG/JPEG via their registered signatures.
	if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	// TGA has no magic number to register, so try it explicitly.
	if img, err := DecodeTGA(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	return nil, fmt.Errorf("decode %s: not a supported PNG/JPEG/TGA image", name)
}

// DecodeFile reads and decodes a texture file from disk.
func DecodeFile(path string) (image.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeImage(path, data)
}

// EncodeFile writes img to path in the given format.
func EncodeFile(path string, img image.Image, format Format) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	switch format {
	case FormatJPEG:
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return err
		}
	case FormatTGA:
		if err := EncodeTGA(f, img); err != nil {
			return err
		}
	default:
		if err := png.Encode(f, img); err != nil {
			return err
		}
	}
	return f.Close()
}

// nextPow2 returns the smallest power of two >= n (n>=1).
func nextPow2(n int) int {
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}

// Resize returns img scaled to w×h using a high-quality Catmull-Rom kernel
// (the closest stdlib-adjacent equivalent to ImageMagick's Lanczos resize the
// shell tools used). If the source is already w×h it is returned unchanged.
func Resize(img image.Image, w, h int) image.Image {
	if img.Bounds().Dx() == w && img.Bounds().Dy() == h {
		return img
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
	return dst
}

// SnapToPowerOfTwo scales img so both dimensions are powers of two, the
// constraint the engine's renderer enforces (it FATALs on non-power-of-two
// textures). Each axis is snapped up to the next power of two independently.
// An image already power-of-two on both axes is returned unchanged.
func SnapToPowerOfTwo(img image.Image) image.Image {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	nw, nh := nextPow2(w), nextPow2(h)
	if nw == w && nh == h {
		return img
	}
	return Resize(img, nw, nh)
}
