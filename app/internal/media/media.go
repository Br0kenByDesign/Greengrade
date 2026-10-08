// Package media re-encodes every uploaded image. Decoding to raw pixels and encoding a fresh
// JPEG guarantees that no metadata (EXIF, GPS, XMP, ICC comments, thumbnails) survives and
// that polyglot or malformed files are neutralised.
package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	MaxUploadBytes = 15 << 20
	maxPixels      = 24_000_000 // 6000x4000, enough for any phone camera; decoding needs ~4 bytes per pixel
	FullEdge       = 2048
	ThumbEdge      = 560
)

// Only a few images are decoded at the same time, so parallel uploads cannot exhaust memory.
var slots = make(chan struct{}, 2)

var ErrBusy = errors.New("Gerade werden viele Fotos verarbeitet. Bitte versuche es in einem Moment noch einmal.")

// Acquire reserves a processing slot (waits up to 20 seconds). Call the returned release when done.
func Acquire(ctx context.Context) (release func(), err error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ErrBusy
	}
}

var ErrUnsupported = errors.New("Nur JPEG- und PNG-Bilder werden unterstützt.")
var ErrTooLarge = errors.New("Das Bild ist zu groß. Erlaubt sind höchstens 15 MB und 24 Megapixel.")

type Result struct {
	Full, Thumb   []byte
	Width, Height int
	Hash          []byte // SHA-256 of the re-encoded full image, used by moderation
}

func Process(r io.Reader) (*Result, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxUploadBytes {
		return nil, ErrTooLarge
	}
	switch http.DetectContentType(data) {
	case "image/jpeg", "image/png":
	default:
		return nil, ErrUnsupported
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrUnsupported
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 12000 || cfg.Height > 12000 || cfg.Width*cfg.Height > maxPixels {
		return nil, ErrTooLarge
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrUnsupported
	}
	rgba := image.NewRGBA(src.Bounds())
	draw.Draw(rgba, rgba.Bounds(), &image.Uniform{C: image.White}, image.Point{}, draw.Src) // flatten transparency
	draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Over)

	full := downscale(rgba, FullEdge)
	thumb := downscale(full, ThumbEdge)
	res := &Result{Width: full.Bounds().Dx(), Height: full.Bounds().Dy()}
	var b1, b2 bytes.Buffer
	if err := jpeg.Encode(&b1, full, &jpeg.Options{Quality: 84}); err != nil {
		return nil, err
	}
	if err := jpeg.Encode(&b2, thumb, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	res.Full, res.Thumb = b1.Bytes(), b2.Bytes()
	sum := sha256.Sum256(res.Full)
	res.Hash = sum[:]
	return res, nil
}

// downscale uses an area-average (box) filter, good quality for reductions.
func downscale(src *image.RGBA, maxEdge int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if sw <= maxEdge && sh <= maxEdge {
		return src
	}
	dw, dh := maxEdge, sh*maxEdge/sw
	if sh > sw {
		dw, dh = sw*maxEdge/sh, maxEdge
	}
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		y0, y1 := y*sh/dh, (y+1)*sh/dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < dw; x++ {
			x0, x1 := x*sw/dw, (x+1)*sw/dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, n uint32
			for yy := y0; yy < y1; yy++ {
				off := yy*src.Stride + x0*4
				for xx := x0; xx < x1; xx++ {
					r += uint32(src.Pix[off])
					g += uint32(src.Pix[off+1])
					b += uint32(src.Pix[off+2])
					off += 4
					n++
				}
			}
			o := y*dst.Stride + x*4
			dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3] = uint8(r/n), uint8(g/n), uint8(b/n), 255
		}
	}
	return dst
}

// Store keeps files at DATA_DIR/photos/<first two chars>/<uuid>.jpg and <uuid>_t.jpg.
type Store struct{ dir string }

func NewStore(dataDir string) (*Store, error) {
	d := filepath.Join(dataDir, "photos")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: d}, nil
}

func (s *Store) path(id string, thumb bool) string {
	name := id + ".jpg"
	if thumb {
		name = id + "_t.jpg"
	}
	return filepath.Join(s.dir, id[:2], name)
}

func writeAtomic(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// id must be a canonical UUID string (validated by the caller via uuid parsing).
func (s *Store) Save(id string, r *Result) error {
	if err := writeAtomic(s.path(id, false), r.Full); err != nil {
		return err
	}
	return writeAtomic(s.path(id, true), r.Thumb)
}

func (s *Store) Open(id string, thumb bool) (*os.File, error) { return os.Open(s.path(id, thumb)) }

func (s *Store) Delete(id string) {
	os.Remove(s.path(id, false))
	os.Remove(s.path(id, true))
}

// Walk lists all stored photo ids (used to remove orphaned files).
func (s *Store) Walk(fn func(id string)) {
	filepath.WalkDir(s.dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		n := d.Name()
		if strings.HasSuffix(n, ".tmp") {
			os.Remove(p)
			return nil
		}
		if strings.HasSuffix(n, ".jpg") && !strings.HasSuffix(n, "_t.jpg") {
			fn(strings.TrimSuffix(n, ".jpg"))
		}
		return nil
	})
}
