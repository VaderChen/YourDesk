//go:build cgo && ffmpeg

package softwarevideo

import (
	"encoding/binary"
	"hash/crc32"
	"os"
	"strings"
	"testing"
)

func TestSoftwareDecodeSmoke(t *testing.T) {
	for _, tc := range []struct {
		name        string
		codec, w, h int
	}{
		{"h264-1080.bin", 1, 1920, 1080}, {"hevc-1080.bin", 2, 1920, 1080},
		{"h264.bin", 1, 128, 128}, {"hevc.bin", 2, 128, 128}, {"h264-1080.annexb", 1, 1920, 1080}, {"hevc-1080.annexb", 2, 1920, 1080},
		{"av1-128.obu", 3, 128, 128}, {"av1-1080.obu", 3, 1920, 1080},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile("../video/probes/" + tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(tc.name, ".bin") {
				var annex []byte
				wire := data[1:]
				for len(wire) > 0 {
					n := int(binary.BigEndian.Uint32(wire))
					wire = wire[4:]
					annex = append(annex, 0, 0, 0, 1)
					annex = append(annex, wire[:n]...)
					wire = wire[n:]
				}
				data = annex
			}
			d, err := New(tc.codec)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			for i := 0; i < 3; i++ {
				im, err := d.Decode(data)
				if err != nil {
					t.Fatal(err)
				}
				if im.Bounds().Dx() != tc.w || im.Bounds().Dy() != tc.h {
					t.Fatalf("尺寸錯誤：%v", im.Bounds())
				}
				r, g, b, a := im.At(tc.w/2, tc.h/2).RGBA()
				if r > 2048 || g > 2048 || b > 2048 || a != 65535 {
					t.Fatalf("黑色測試影格顏色錯誤：%d %d %d %d", r, g, b, a)
				}
			}
			if d.DecodingMode() != "software" {
				t.Fatal("未強制軟解")
			}
		})
	}
}
func TestSoftwareDecodeRejectsInvalid(t *testing.T) {
	for _, codec := range []int{1, 2, 3} {
		d, err := New(codec)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = d.Decode([]byte{0xff, 0xff, 0}); err == nil {
			t.Fatal("接受損毀資料")
		}
		d.Close()
		d.Close()
		if _, err = d.Decode([]byte{1}); err == nil {
			t.Fatal("已關閉仍可使用")
		}
	}
}

// 每個工作階段依序處理 I/P/P，不能每張重建 decoder 或偷偷退回硬解。
func TestSoftwareDecodeGOP(t *testing.T) {
	for i, name := range []string{"h264", "hevc", "av1"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + name + ".frames")
			if err != nil {
				t.Fatal(err)
			}
			d, err := New(i + 1)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			count := 0
			var first, last uint32
			for len(data) > 0 {
				if len(data) < 4 {
					t.Fatal("fixture 截斷")
				}
				n := int(binary.BigEndian.Uint32(data))
				data = data[4:]
				if n <= 0 || n > len(data) {
					t.Fatal("fixture 長度錯誤")
				}
				im, err := d.Decode(data[:n])
				if err != nil {
					t.Fatalf("frame %d: %v", count, err)
				}
				if im.Bounds().Dx() != 128 || im.Bounds().Dy() != 128 {
					t.Fatal(im.Bounds())
				}
				last = crc32.ChecksumIEEE(im.Pix)
				if count == 0 {
					first = last
				}
				count++
				data = data[n:]
			}
			if count != 3 || first == last {
				t.Fatal("未輸出三張不同的 GOP 影格")
			}
		})
	}
}
