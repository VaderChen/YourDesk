package desktop

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"testing"
)

type deltaTestJPEG struct {
	calls  int
	failAt int
}

func (e *deltaTestJPEG) Encode(image.Image, int) ([]byte, error) {
	e.calls++
	if e.calls == e.failAt {
		return nil, errors.New("injected patch failure")
	}
	return nil, nil
}

func TestDeltaPartialFailurePreservesCommittedFrame(t *testing.T) {
	for _, budget := range []int{0, 64 << 20} {
		t.Run(fmt.Sprintf("budget=%d", budget), func(t *testing.T) {
			// An offset subimage also exercises row padding and non-zero origins.
			parent := image.NewRGBA(image.Rect(7, 9, 547, 552))
			src := parent.SubImage(image.Rect(11, 13, 523, 525)).(*image.RGBA)
			codec := &deltaTestJPEG{}
			e := DeltaEncoder{Encoder: codec, CompareBytes: budget}
			if _, err := e.Encode(src, false); err != nil {
				t.Fatal(err)
			}
			src.SetRGBA(12, 14, color.RGBA{R: 255, A: 255})
			src.SetRGBA(400, 400, color.RGBA{B: 255, A: 255})
			codec.failAt = codec.calls + 2
			if _, err := e.Encode(src, false); err == nil {
				t.Fatal("second patch did not fail")
			}
			patches, err := e.Encode(src, false)
			if err != nil || len(patches) != 2 || patches[0].Keyframe || patches[1].Keyframe {
				t.Fatalf("retry lost a patch: patches=%+v err=%v", patches, err)
			}
			if patches[0].X != 0 || patches[0].Y != 0 || patches[1].X != 384 || patches[1].Y != 384 {
				t.Fatalf("wrong patch coordinates: %+v", patches)
			}
			if unchanged, err := e.Encode(src, false); err != nil || len(unchanged) != 0 {
				t.Fatalf("committed frame changed: patches=%+v err=%v", unchanged, err)
			}
			// Revert only the first tile; its committed baseline must be independent.
			src.SetRGBA(12, 14, color.RGBA{})
			patches, err = e.Encode(src, false)
			if err != nil || len(patches) != 1 || patches[0].X != 0 || patches[0].Y != 0 || patches[0].Keyframe {
				t.Fatalf("reverted tile lost: patches=%+v err=%v", patches, err)
			}
			if budget > 0 {
				for y := 0; y < src.Bounds().Dy(); y++ {
					i := src.PixOffset(src.Rect.Min.X, src.Rect.Min.Y+y)
					j := e.previousRGBA.PixOffset(0, y)
					if !bytes.Equal(src.Pix[i:i+512*4], e.previousRGBA.Pix[j:j+512*4]) {
						t.Fatalf("committed pixels differ on row %d", y)
					}
				}
			}
		})
	}
}

func TestDeltaWorkspaceAcrossFrameChanges(t *testing.T) {
	for _, budget := range []int{0, 64 << 20} {
		e := DeltaEncoder{Encoder: &deltaTestJPEG{}, CompareBytes: budget}
		for _, size := range []image.Point{{512, 384}, {129, 257}, {512, 384}, {1, 1}} {
			src := image.NewRGBA(image.Rectangle{Max: size})
			patches, err := e.Encode(src, false)
			if err != nil || len(patches) != 1 || !patches[0].Keyframe || patches[0].Width != size.X || patches[0].Height != size.Y {
				t.Fatalf("budget=%d size=%v reset: patches=%+v err=%v", budget, size, patches, err)
			}
			if patches, err = e.Encode(src, false); err != nil || len(patches) != 0 {
				t.Fatalf("budget=%d size=%v unchanged: patches=%+v err=%v", budget, size, patches, err)
			}
			if patches, err = e.Encode(src, true); err != nil || len(patches) != 1 || !patches[0].Keyframe {
				t.Fatalf("budget=%d size=%v force: patches=%+v err=%v", budget, size, patches, err)
			}
		}
	}
}

func TestPatchImageKeepsSourceBounds(t *testing.T) {
	src := image.NewRGBA(image.Rect(11, 13, 270, 278))
	for _, region := range []image.Rectangle{image.Rect(0, 0, 259, 265), image.Rect(128, 128, 259, 265)} {
		view := patchImage(src, region).(*image.RGBA)
		if src.Bounds() != image.Rect(11, 13, 270, 278) || view.Bounds() != image.Rect(0, 0, region.Dx(), region.Dy()) {
			t.Fatalf("source/view bounds changed: source=%v view=%v", src.Bounds(), view.Bounds())
		}
		if &view.Pix[0] != &src.Pix[src.PixOffset(src.Rect.Min.X+region.Min.X, src.Rect.Min.Y+region.Min.Y)] {
			t.Fatal("view no longer shares source pixels")
		}
	}
}

func TestDeltaUnchangedDoesNotAllocate(t *testing.T) {
	for _, budget := range []int{0, 64 << 20} {
		src := image.NewRGBA(image.Rect(0, 0, 384, 256))
		e := DeltaEncoder{Encoder: &deltaTestJPEG{}, CompareBytes: budget}
		if _, err := e.Encode(src, false); err != nil {
			t.Fatal(err)
		}
		allocs := testing.AllocsPerRun(20, func() {
			patches, err := e.Encode(src, false)
			if err != nil || len(patches) != 0 {
				t.Fatalf("unchanged frame: patches=%+v err=%v", patches, err)
			}
		})
		if allocs != 0 {
			t.Fatalf("budget=%d unchanged frame allocated %.1f objects", budget, allocs)
		}
	}
}

func BenchmarkDeltaDetection(b *testing.B) {
	for _, size := range []image.Point{{1920, 1080}, {3840, 2160}} {
		for _, mode := range []string{"compare", "hash"} {
			for _, changed := range []bool{false, true} {
				name := fmt.Sprintf("%dx%d/%s/changed=%t", size.X, size.Y, mode, changed)
				b.Run(name, func(b *testing.B) {
					src := image.NewRGBA(image.Rectangle{Max: size})
					e := DeltaEncoder{Encoder: &deltaTestJPEG{}}
					if mode == "compare" {
						e.CompareBytes = 64 << 20
					}
					if _, err := e.Encode(src, false); err != nil {
						b.Fatal(err)
					}
					// Warm both hash buffers before measuring steady-state frames.
					if _, err := e.Encode(src, false); err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if changed {
							src.Pix[0] ^= 1
						}
						patches, err := e.Encode(src, false)
						if err != nil || (changed && len(patches) != 1) || (!changed && len(patches) != 0) {
							b.Fatalf("patches=%d err=%v", len(patches), err)
						}
					}
				})
			}
		}
	}
}
