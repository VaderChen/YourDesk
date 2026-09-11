package desktop

import (
	"bytes"
	"hash/fnv"
	"image"
	"image/draw"
	"image/jpeg"
)

const DefaultTileSize = 128

type Patch struct {
	X, Y          int
	Width, Height int
	JPEG          []byte
	Keyframe      bool
}

type DeltaEncoder struct {
	TileSize      int
	Quality       int
	Encoder       JPEGEncoder
	previous      []uint64
	width, height int
}

type JPEGEncoder interface {
	Encode(image.Image, int) ([]byte, error)
}

// Encode returns only changed horizontal tile runs. When too much of the
// desktop changed, a single full-frame JPEG is cheaper and is used instead.
func (e *DeltaEncoder) Encode(src image.Image, forceKeyframe bool) ([]Patch, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	tile := e.TileSize
	if tile <= 0 {
		tile = DefaultTileSize
	}
	cols := (w + tile - 1) / tile
	rows := (h + tile - 1) / tile
	hashes := make([]uint64, cols*rows)
	changed := make([]bool, len(hashes))
	count := 0
	reset := w != e.width || h != e.height || len(e.previous) != len(hashes)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			r := image.Rect(x*tile, y*tile, min((x+1)*tile, w), min((y+1)*tile, h))
			sum := hashRegion(src, r)
			i := y*cols + x
			hashes[i] = sum
			if reset || forceKeyframe || sum != e.previous[i] {
				changed[i] = true
				count++
			}
		}
	}
	e.previous, e.width, e.height = hashes, w, h
	if count == 0 {
		return nil, nil
	}
	if reset || forceKeyframe || count*100 >= len(changed)*35 {
		p, err := e.encodePatch(src, image.Rect(0, 0, w, h), true)
		return []Patch{p}, err
	}
	patches := make([]Patch, 0, count)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; {
			if !changed[y*cols+x] {
				x++
				continue
			}
			start := x
			for x < cols && changed[y*cols+x] {
				x++
			}
			r := image.Rect(start*tile, y*tile, min(x*tile, w), min((y+1)*tile, h))
			p, err := e.encodePatch(src, r, false)
			if err != nil {
				return nil, err
			}
			patches = append(patches, p)
		}
	}
	return patches, nil
}

func (e *DeltaEncoder) encodePatch(src image.Image, r image.Rectangle, key bool) (Patch, error) {
	if e.Encoder == nil {
		return encodePatch(src, r, e.Quality, key)
	}
	rgba := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, r.Min, draw.Src)
	b, err := e.Encoder.Encode(rgba, e.Quality)
	return Patch{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy(), JPEG: b, Keyframe: key}, err
}

func encodePatch(src image.Image, r image.Rectangle, quality int, key bool) (Patch, error) {
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}
	rgba := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, r.Min, draw.Src)
	var out bytes.Buffer
	err := jpeg.Encode(&out, rgba, &jpeg.Options{Quality: quality})
	return Patch{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy(), JPEG: out.Bytes(), Keyframe: key}, err
}
func hashRegion(src image.Image, r image.Rectangle) uint64 {
	h := fnv.New64a()
	var px [4]byte
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			rr, g, b, a := src.At(x, y).RGBA()
			px[0] = byte(rr >> 8)
			px[1] = byte(g >> 8)
			px[2] = byte(b >> 8)
			px[3] = byte(a >> 8)
			_, _ = h.Write(px[:])
		}
	}
	return h.Sum64()
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
