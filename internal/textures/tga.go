package textures

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
)

// This file implements the minimal Targa (TGA) support the texture pipeline
// needs. The Jedi Outcast asset paks store many surface textures as TGA, and
// the engine's loader reads uncompressed and RLE-compressed truecolour Targas
// (see tr_image.cpp LoadTGA). Go's standard library has no TGA codec, so we
// decode both the uncompressed (type 2) and run-length-encoded (type 10)
// truecolour variants here, and always *write* uncompressed truecolour — the
// form the loader reads most reliably and the shell tools produced via
// `convert -define tga:compression=none`.

// tgaHeader is the fixed 18-byte Targa file header.
type tgaHeader struct {
	IDLength     uint8
	ColorMapType uint8
	ImageType    uint8
	// Colour-map spec (unused for truecolour, but present in the header).
	CMapLength uint16
	CMapDepth  uint8
	// Image spec.
	Width   uint16
	Height  uint16
	Depth   uint8 // bits per pixel: 24 or 32
	Descrip uint8 // bit5 = top-left origin; low nibble = alpha bits
}

// DecodeTGA reads an uncompressed or RLE truecolour (24/32-bit) Targa image.
// It mirrors what the engine's loader accepts; palette-indexed and grayscale
// Targas (not used by the retail surface textures) are rejected.
func DecodeTGA(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(data) < 18 {
		return nil, errors.New("tga: file too short for header")
	}
	var h tgaHeader
	h.IDLength = data[0]
	h.ColorMapType = data[1]
	h.ImageType = data[2]
	h.CMapLength = binary.LittleEndian.Uint16(data[5:])
	h.CMapDepth = data[7]
	h.Width = binary.LittleEndian.Uint16(data[12:])
	h.Height = binary.LittleEndian.Uint16(data[14:])
	h.Depth = data[16]
	h.Descrip = data[17]

	if h.ImageType != 2 && h.ImageType != 10 {
		return nil, fmt.Errorf("tga: unsupported image type %d (only truecolour 2/10)", h.ImageType)
	}
	if h.Depth != 24 && h.Depth != 32 {
		return nil, fmt.Errorf("tga: unsupported bit depth %d (only 24/32)", h.Depth)
	}
	if h.Width == 0 || h.Height == 0 {
		return nil, errors.New("tga: zero dimension")
	}

	// Skip the image ID and any colour map (present but unused for truecolour).
	off := 18 + int(h.IDLength)
	if h.ColorMapType == 1 {
		off += int(h.CMapLength) * ((int(h.CMapDepth) + 7) / 8)
	}
	if off > len(data) {
		return nil, errors.New("tga: header extends past end of file")
	}

	w, hgt := int(h.Width), int(h.Height)
	bytesPerPixel := int(h.Depth) / 8
	img := image.NewNRGBA(image.Rect(0, 0, w, hgt))

	// Targa stores bottom-up unless descriptor bit 5 is set (top-down).
	topDown := h.Descrip&0x20 != 0

	putPixel := func(x, y int, px []byte) {
		row := y
		if !topDown {
			row = hgt - 1 - y
		}
		// TGA truecolour is stored BGR(A).
		b, g, r := px[0], px[1], px[2]
		a := uint8(0xFF)
		if bytesPerPixel == 4 {
			a = px[3]
		}
		img.SetNRGBA(x, row, color.NRGBA{R: r, G: g, B: b, A: a})
	}

	pixels := data[off:]
	if h.ImageType == 2 {
		need := w * hgt * bytesPerPixel
		if len(pixels) < need {
			return nil, errors.New("tga: pixel data truncated")
		}
		idx := 0
		for y := range hgt {
			for x := range w {
				putPixel(x, y, pixels[idx:idx+bytesPerPixel])
				idx += bytesPerPixel
			}
		}
		return img, nil
	}

	// RLE (type 10): packets of a 1-byte count then either one pixel repeated
	// (RLE packet) or count literal pixels (raw packet). Packets may cross
	// scanline boundaries, so decode into a flat pixel stream.
	idx := 0
	total := w * hgt
	done := 0
	for done < total {
		if idx >= len(pixels) {
			return nil, errors.New("tga: RLE data truncated")
		}
		packet := pixels[idx]
		idx++
		count := int(packet&0x7F) + 1
		if done+count > total {
			return nil, errors.New("tga: RLE packet overruns image")
		}
		if packet&0x80 != 0 {
			// RLE packet: one pixel repeated count times.
			if idx+bytesPerPixel > len(pixels) {
				return nil, errors.New("tga: RLE pixel truncated")
			}
			px := pixels[idx : idx+bytesPerPixel]
			idx += bytesPerPixel
			for i := range count {
				p := done + i
				putPixel(p%w, p/w, px)
			}
		} else {
			// Raw packet: count literal pixels.
			if idx+count*bytesPerPixel > len(pixels) {
				return nil, errors.New("tga: raw packet truncated")
			}
			for i := range count {
				p := done + i
				putPixel(p%w, p/w, pixels[idx:idx+bytesPerPixel])
				idx += bytesPerPixel
			}
		}
		done += count
	}
	return img, nil
}

// EncodeTGA writes img as an uncompressed 32-bit truecolour, top-down Targa —
// the form the engine reads reliably and the equivalent of the shell tools'
// `convert -define tga:compression=none`.
func EncodeTGA(w io.Writer, img image.Image) error {
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	if width == 0 || height == 0 {
		return errors.New("tga: cannot encode empty image")
	}
	if width > 0xFFFF || height > 0xFFFF {
		return fmt.Errorf("tga: dimensions %dx%d exceed 16-bit limit", width, height)
	}

	var hdr [18]byte
	hdr[2] = 2 // uncompressed truecolour
	// width and height are bounded to [1, 0xFFFF] by the guard above.
	binary.LittleEndian.PutUint16(hdr[12:], uint16(width))  //nolint:gosec // bounded above
	binary.LittleEndian.PutUint16(hdr[14:], uint16(height)) //nolint:gosec // bounded above
	hdr[16] = 32                                            // 32 bpp
	hdr[17] = 0x28                                          // top-down origin (bit5) + 8 alpha bits (low nibble)
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}

	row := make([]byte, width*4)
	for y := range height {
		for x := range width {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			i := x * 4
			// BGRA on disk.
			row[i+0] = c.B
			row[i+1] = c.G
			row[i+2] = c.R
			row[i+3] = c.A
		}
		if _, err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}
