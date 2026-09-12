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
	CompareBytes  int // 偵測策略允許的前一畫面快照預算；0 使用完整雜湊。
	previousRGBA  *image.RGBA
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
	rgba, direct := src.(*image.RGBA)
	compare := direct && e.CompareBytes > 0 && int64(w)*int64(h)*4 <= int64(e.CompareBytes)
	if compare && (e.previousRGBA == nil || e.previousRGBA.Bounds().Size() != b.Size()) {
		reset = true
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			r := image.Rect(x*tile, y*tile, min((x+1)*tile, w), min((y+1)*tile, h))
			sourceRegion := r.Add(b.Min)
			i := y*cols + x
			different := reset || forceKeyframe
			if compare {
				if !different {
					different = !equalRegion(rgba, e.previousRGBA, sourceRegion, r)
				}
			} else {
				hashes[i] = hashRegion(src, sourceRegion)
				different = different || hashes[i] != e.previous[i]
			}
			if different {
				changed[i] = true
				count++
			}
		}
	}
	// 編碼全部成功後才提交基準；失敗重試不可漏掉尚未送出的區塊。
	commit := func() {
		e.previous, e.width, e.height = hashes, w, h
		if compare {
			if e.previousRGBA == nil || e.previousRGBA.Bounds().Size() != b.Size() {
				e.previousRGBA = image.NewRGBA(image.Rect(0, 0, w, h))
			}
			for y := 0; y < h; y++ {
				i := rgba.PixOffset(b.Min.X, b.Min.Y+y)
				copy(e.previousRGBA.Pix[y*e.previousRGBA.Stride:y*e.previousRGBA.Stride+w*4], rgba.Pix[i:i+w*4])
			}
		} else {
			e.previousRGBA = nil
		}
	}
	if count == 0 {
		return nil, nil
	}
	if reset || forceKeyframe || count*100 >= len(changed)*35 {
		p, err := e.encodePatch(src, image.Rect(0, 0, w, h), true)
		if err == nil {
			commit()
		}
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
	commit()
	return patches, nil
}

func (e *DeltaEncoder) encodePatch(src image.Image, r image.Rectangle, key bool) (Patch, error) {
	if e.Encoder == nil {
		return encodePatch(src, r, e.Quality, key)
	}
	region := patchImage(src, r)
	b, err := e.Encoder.Encode(region, e.Quality)
	return Patch{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy(), JPEG: b, Keyframe: key}, err
}

func encodePatch(src image.Image, r image.Rectangle, quality int, key bool) (Patch, error) {
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}
	region := patchImage(src, r)
	var out bytes.Buffer
	err := jpeg.Encode(&out, region, &jpeg.Options{Quality: quality})
	return Patch{X: r.Min.X, Y: r.Min.Y, Width: r.Dx(), Height: r.Dy(), JPEG: out.Bytes(), Keyframe: key}, err
}
func hashRegion(src image.Image, r image.Rectangle) uint64 {
	h := fnv.New64a()
	if rgba, ok := src.(*image.RGBA); ok {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			i := rgba.PixOffset(r.Min.X, y)
			_, _ = h.Write(rgba.Pix[i : i+r.Dx()*4])
		}
		return h.Sum64()
	}
	var px [4]byte
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
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

// 只讀的零起點 view 保留來源 stride，編碼器須在 Encode 返回前消費像素。
func patchImage(src image.Image, r image.Rectangle) image.Image {
	region := r.Add(src.Bounds().Min)
	if rgba, ok := src.(*image.RGBA); ok {
		sub := rgba.SubImage(region).(*image.RGBA)
		return &image.RGBA{Pix: sub.Pix, Stride: sub.Stride, Rect: image.Rect(0, 0, r.Dx(), r.Dy())}
	}
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(out, out.Bounds(), src, region.Min, draw.Src)
	return out
}
func equalRegion(a, b *image.RGBA, ar, br image.Rectangle) bool {
	for y := 0; y < ar.Dy(); y++ {
		i, j := a.PixOffset(ar.Min.X, ar.Min.Y+y), b.PixOffset(br.Min.X, br.Min.Y+y)
		if !bytes.Equal(a.Pix[i:i+ar.Dx()*4], b.Pix[j:j+br.Dx()*4]) {
			return false
		}
	}
	return true
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
