package video

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

type testJPEGEncoder struct {
	hardware bool
	backend  string
	err      error
	calls    int
	closed   int
}

func (e *testJPEGEncoder) Encode(image.Image, int) ([]byte, error) {
	e.calls++
	if e.err != nil {
		return nil, e.err
	}
	return []byte("test-jpeg"), nil
}
func (e *testJPEGEncoder) Backend() string { return e.backend }
func (e *testJPEGEncoder) Hardware() bool  { return e.hardware }
func (e *testJPEGEncoder) Close() error {
	e.closed++
	return nil
}

type testJPEGDecoder struct {
	hardware bool
	backend  string
	err      error
	calls    int
	closed   int
	result   image.Image
}

func (d *testJPEGDecoder) Decode([]byte) (image.Image, error) {
	d.calls++
	if d.err != nil {
		return nil, d.err
	}
	return d.result, nil
}
func (d *testJPEGDecoder) Backend() string { return d.backend }
func (d *testJPEGDecoder) Hardware() bool  { return d.hardware }
func (d *testJPEGDecoder) Close() error {
	d.closed++
	return nil
}

func TestSoftwareJPEGCodecRoundTrip(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 6))
	src.Set(3, 2, color.RGBA{R: 250, G: 30, B: 20, A: 255})

	enc, encSelection := NewJPEGEncoderForCodec(CodecSoftwareJPEG)
	if enc.Hardware() || encSelection.Selected != CodecSoftwareJPEG {
		t.Fatalf("encoder selection = %+v", encSelection)
	}
	encoded, err := enc.Encode(src, 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) < 4 || !bytes.Equal(encoded[:2], []byte{0xff, 0xd8}) {
		n := minInt(4, len(encoded))
		t.Fatalf("not a JPEG: %x", encoded[:n])
	}

	dec, decSelection := NewJPEGDecoderForCodec(string(CodecSoftwareJPEG))
	if dec.Hardware() || decSelection.Codec != CodecSoftwareJPEG {
		t.Fatalf("decoder selection = %+v", decSelection)
	}
	decoded, err := dec.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 8 || decoded.Bounds().Dy() != 6 {
		t.Fatalf("decoded bounds = %v", decoded.Bounds())
	}
	_ = enc.Close()
	_ = dec.Close()
}

func TestFallbackEncoderRetriesFrameAndStaysSoftware(t *testing.T) {
	hw := &testJPEGEncoder{hardware: true, backend: "test-hardware", err: errors.New("hardware failed")}
	sw := &testJPEGEncoder{backend: "test-software"}
	enc := &fallbackEncoder{current: hw, fallback: sw}

	got, err := enc.Encode(image.NewRGBA(image.Rect(0, 0, 1, 1)), 70)
	if err != nil || string(got) != "test-jpeg" {
		t.Fatalf("first Encode() = %q, %v", got, err)
	}
	if hw.calls != 1 || hw.closed != 1 || sw.calls != 1 || enc.Hardware() {
		t.Fatalf("after fallback: hw calls=%d close=%d, sw calls=%d, hardware=%v", hw.calls, hw.closed, sw.calls, enc.Hardware())
	}
	_, err = enc.Encode(image.NewRGBA(image.Rect(0, 0, 1, 1)), 70)
	if err != nil || hw.calls != 1 || sw.calls != 2 {
		t.Fatalf("second Encode: err=%v hw=%d sw=%d", err, hw.calls, sw.calls)
	}
}

func TestFallbackDecoderRetriesFrameAndStaysSoftware(t *testing.T) {
	want := image.NewRGBA(image.Rect(0, 0, 2, 3))
	hw := &testJPEGDecoder{hardware: true, backend: "test-hardware", err: errors.New("hardware failed")}
	sw := &testJPEGDecoder{backend: "test-software", result: want}
	dec := &fallbackDecoder{current: hw, fallback: sw}

	got, err := dec.Decode([]byte("jpeg"))
	if err != nil || got != want {
		t.Fatalf("first Decode() = %v, %v", got, err)
	}
	if hw.calls != 1 || hw.closed != 1 || sw.calls != 1 || dec.Hardware() {
		t.Fatalf("after fallback: hw calls=%d close=%d, sw calls=%d, hardware=%v", hw.calls, hw.closed, sw.calls, dec.Hardware())
	}
	_, err = dec.Decode([]byte("jpeg"))
	if err != nil || hw.calls != 1 || sw.calls != 2 {
		t.Fatalf("second Decode: err=%v hw=%d sw=%d", err, hw.calls, sw.calls)
	}
}

func TestExplicitSoftwareJPEGIsValidStandardJPEG(t *testing.T) {
	enc, _ := NewJPEGEncoderForCodec(CodecSoftwareJPEG)
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.White)
		}
	}
	b, err := enc.Encode(img, 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jpeg.Decode(bytes.NewReader(b)); err != nil {
		t.Fatalf("image/jpeg cannot decode result: %v", err)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
