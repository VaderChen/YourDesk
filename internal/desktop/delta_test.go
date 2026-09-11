package desktop

import (
	"image"
	"image/color"
	"testing"
)

type recordingJPEGEncoder struct {
	calls   int
	quality int
	bounds  image.Rectangle
}

func (e *recordingJPEGEncoder) Encode(img image.Image, quality int) ([]byte, error) {
	e.calls++
	e.quality = quality
	e.bounds = img.Bounds()
	return []byte("encoded-by-injected-codec"), nil
}

func TestDeltaEncoderOnlySendsChangedRegion(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	e := DeltaEncoder{TileSize: 128, Quality: 70}
	first, err := e.Encode(img, false)
	if err != nil || len(first) != 1 || !first[0].Keyframe {
		t.Fatalf("first=%d err=%v", len(first), err)
	}
	none, err := e.Encode(img, false)
	if err != nil || len(none) != 0 {
		t.Fatalf("unchanged=%d err=%v", len(none), err)
	}
	img.Set(10, 10, color.White)
	changed, err := e.Encode(img, false)
	if err != nil || len(changed) != 1 || changed[0].Keyframe {
		t.Fatalf("changed=%d err=%v", len(changed), err)
	}
}

func TestDeltaEncoderUsesInjectedJPEGEncoder(t *testing.T) {
	codec := &recordingJPEGEncoder{}
	e := DeltaEncoder{TileSize: 128, Quality: 73, Encoder: codec}
	patches, err := e.Encode(image.NewRGBA(image.Rect(0, 0, 32, 24)), false)
	if err != nil {
		t.Fatal(err)
	}
	if codec.calls != 1 || codec.quality != 73 || codec.bounds != image.Rect(0, 0, 32, 24) {
		t.Fatalf("codec calls=%d quality=%d bounds=%v", codec.calls, codec.quality, codec.bounds)
	}
	if len(patches) != 1 || string(patches[0].JPEG) != "encoded-by-injected-codec" {
		t.Fatalf("patches = %+v", patches)
	}
}
