//go:build windows && cgo && ffmpeg

package video

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
	"yourdesk/internal/softwarevideo"
)

// 在 Windows GPU 上比較同一份 BT.709 AV1 的硬／軟解彩色色塊。
// 灰階不能區分 601/709；缺少 AV1 硬解時明確 skip，不把 CPU 備援當硬解通過。
func TestWindowsHardwareAV1ColorRoundTrip(t *testing.T) {
	encoder, err := NewIntraEncoder(CodecSoftwareAV1)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	colors := []color.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 255}}
	for i, c := range colors {
		x, y := (i%2)*64, (i/2)*64
		draw.Draw(src, image.Rect(x, y, x+64, y+64), image.NewUniform(c), image.Point{}, draw.Src)
	}
	data, err := encoder.Encode(src, 95)
	if err != nil {
		t.Fatal(err)
	}
	var hardware windowsDecoder
	hardware.codec = WireAV1
	defer hardware.Close()
	actual, err := hardware.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if hardware.DecodingMode() != "hardware" {
		t.Skip("AV1 GPU 解碼不可用，不能用 CPU 備援宣稱硬體色彩測試通過")
	}
	cpu, err := softwarevideo.New(3)
	if err != nil {
		t.Fatal(err)
	}
	defer cpu.Close()
	expected, err := cpu.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []image.Point{{32, 32}, {96, 32}, {32, 96}, {96, 96}} {
		got := color.RGBAModel.Convert(actual.At(p.X, p.Y)).(color.RGBA)
		want := expected.RGBAAt(p.X, p.Y)
		for i, pair := range [][2]byte{{got.R, want.R}, {got.G, want.G}, {got.B, want.B}} {
			difference := int(pair[0]) - int(pair[1])
			if difference < -12 || difference > 12 {
				t.Fatalf("硬／軟解色彩不一致 point=%v channel=%d gpu=%v cpu=%v", p, i, got, want)
			}
		}
	}
}
