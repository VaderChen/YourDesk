//go:build cgo && ffmpeg && (darwin || windows)

package video

import (
	"image"
	"testing"
	"yourdesk/internal/softwarevideo"
)

func TestAV1SoftwareRoundTrip(t *testing.T) {
	for _, size := range []image.Point{{128, 128}, {1920, 1080}} {
		t.Run(size.String(), func(t *testing.T) {
			e, err := NewIntraEncoder(CodecSoftwareAV1)
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			e.(GOPEncoder).SetKeyframeInterval(10)
			e.(RateEncoder).SetRate(2000000, 30)
			d, err := softwarevideo.New(3)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			for n := 0; n < 3; n++ {
				src := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
				for i := 0; i < len(src.Pix); i += 4 {
					src.Pix[i] = byte(30 + n*5)
					src.Pix[i+1] = 120
					src.Pix[i+2] = 200
					src.Pix[i+3] = 255
				}
				data, err := e.Encode(src, 70)
				if err != nil {
					t.Fatal(err)
				}
				key, err := IsKeyframe(WireAV1, data)
				if err != nil || (n == 0 && !key) {
					t.Fatalf("frame %d key=%v: %v", n, key, err)
				}
				decoded, err := d.Decode(data)
				if err != nil || decoded.Bounds() != src.Bounds() {
					t.Fatalf("frame %d: %v", n, err)
				}
				c := decoded.RGBAAt(size.X/2, size.Y/2)
				if c.B < 170 || c.G < 90 || c.R > 80 {
					t.Fatalf("色彩錯誤：%v", c)
				}
			}
		})
	}
}
func TestAV1ReceiverRoundTrip(t *testing.T) {
	e, err := NewIntraEncoder(CodecSoftwareAV1)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.(GOPEncoder).SetKeyframeInterval(10)
	d, err := newIntraDecoder(WireAV1)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for n := 0; n < 3; n++ {
		src := image.NewRGBA(image.Rect(0, 0, 128, 128))
		data, err := e.Encode(src, 70)
		if err != nil {
			t.Fatal(err)
		}
		im, err := d.Decode(data)
		if err != nil || im.Bounds() != src.Bounds() {
			t.Fatalf("frame %d: %v", n, err)
		}
	}
	if r, ok := d.(interface{ DecodingMode() string }); ok {
		t.Log("AV1 接收後端：", r.DecodingMode())
	}
}

func TestAV1ReconfigureAndClose(t *testing.T) {
	e, err := NewIntraEncoder(CodecSoftwareAV1)
	if err != nil {
		t.Fatal(err)
	}
	e.(GOPEncoder).SetKeyframeInterval(10)
	for _, size := range []image.Point{{128, 128}, {130, 128}, {127, 129}} {
		d, err := softwarevideo.New(3)
		if err != nil {
			t.Fatal(err)
		}
		src := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
		data, err := e.Encode(src, 70)
		if err != nil {
			t.Fatal(err)
		}
		key, err := IsKeyframe(WireAV1, data)
		if err != nil || !key {
			t.Fatalf("重設後不是 keyframe：%v", err)
		}
		im, err := d.Decode(data)
		d.Close()
		if err != nil || im.Bounds().Dx() != (size.X+1)&^1 || im.Bounds().Dy() != (size.Y+1)&^1 {
			t.Fatalf("重設尺寸失敗：%v", err)
		}
	}
	e.Close()
	e.Close()
	if _, err := e.Encode(image.NewRGBA(image.Rect(0, 0, 128, 128)), 70); err == nil {
		t.Fatal("關閉後仍可編碼")
	}
}
