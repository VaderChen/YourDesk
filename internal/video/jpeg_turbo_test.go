//go:build cgo && turbojpeg

package video

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestTurboJPEGNativeSmoke(t *testing.T) {
	selected, selection := NewJPEGEncoderForCodec(CodecSoftwareJPEG)
	if selection.Backend != "TurboJPEG CPU SIMD" || selected.Backend() != selection.Backend {
		t.Fatal("正式 JPEG 工廠未選到 TurboJPEG")
	}
	selected.Close()
	enc := newTurboJPEGEncoder()
	dec := newTurboJPEGDecoder()
	if enc == nil || dec == nil {
		t.Fatal("TurboJPEG 未建立")
	}
	defer enc.Close()
	defer dec.Close()
	src := image.NewRGBA(image.Rect(0, 0, 260, 140))
	for y := 0; y < 140; y++ {
		for x := 0; x < 260; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 80, 255})
		}
	}
	sub := src.SubImage(image.Rect(3, 5, 132, 134))
	data, err := enc.Encode(sub, 80)
	if err != nil {
		t.Fatal(err)
	}
	im, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil || im.Bounds().Size() != image.Pt(129, 129) {
		t.Fatalf("Go 不相容 %v", err)
	}
	first, err := dec.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), first.(*image.RGBA).Pix...)
	if _, err = dec.Decode(data); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, first.(*image.RGBA).Pix) {
		t.Fatal("解碼覆寫前一張")
	}
	if _, err = dec.Decode([]byte("broken")); err == nil {
		t.Fatal("接受毀損 JPEG")
	}
	if enc.Hardware() || dec.Hardware() {
		t.Fatal("CPU SIMD 誤報成硬體引擎")
	}
}

func TestTurboCPUFailureFallsBack(t *testing.T) {
	failed := &testJPEGEncoder{backend: "TurboJPEG CPU SIMD", err: errors.New("合成 CPU 後端失敗")}
	backup := &testJPEGEncoder{backend: "Go image/jpeg"}
	e := &fallbackEncoder{current: failed, fallback: backup}
	defer e.Close()
	if _, err := e.Encode(image.NewRGBA(image.Rect(0, 0, 8, 8)), 70); err != nil {
		t.Fatal(err)
	}
	if e.Backend() != backup.Backend() || failed.closed != 1 {
		t.Fatal("CPU 加速失敗未回退")
	}
}
