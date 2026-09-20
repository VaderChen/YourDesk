//go:build darwin && cgo

package video

import (
	"bytes"
	"image"
	"testing"
)

func TestNativeDecodedPixelsSurviveReconfigureAndClose(t *testing.T) {
	for _, codec := range []Codec{CodecHardwareH264, CodecHardwareHEVC} {
		t.Run(string(codec), func(t *testing.T) {
			encoder, err := NewIntraEncoder(codec)
			if err != nil {
				t.Fatal(err)
			}
			defer encoder.Close()
			var decoder DecodeSession
			defer decoder.Close()
			var held []*image.RGBA
			var snapshots [][]byte
			for i := 0; i < 12; i++ {
				w := 128
				if i >= 6 {
					w = 256
				}
				src := image.NewRGBA(image.Rect(0, 0, w, 128))
				for p := 0; p < len(src.Pix); p += 4 {
					src.Pix[p], src.Pix[p+1], src.Pix[p+2], src.Pix[p+3] = byte(20+i*10), 90, 180, 255
				}
				data, err := encoder.Encode(src, 80)
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := decoder.Decode(WireForCodec(codec), data)
				if err != nil {
					t.Fatal(err)
				}
				rgba, ok := decoded.(*image.RGBA)
				if !ok || rgba.Bounds() != src.Bounds() {
					t.Fatal("unexpected decoded format or dimensions")
				}
				pixel := rgba.RGBAAt(w/2, 64)
				if pixel.B < 150 || pixel.G < 60 || pixel.A != 255 {
					t.Fatalf("incorrect pixel conversion: %v", pixel)
				}
				held = append(held, rgba)
				snapshots = append(snapshots, bytes.Clone(rgba.Pix))
			}
			decoder.Close()
			for i := range held {
				if !bytes.Equal(held[i].Pix, snapshots[i]) {
					t.Fatalf("decoded frame %d overwritten after reuse/close", i)
				}
			}
		})
	}
}
