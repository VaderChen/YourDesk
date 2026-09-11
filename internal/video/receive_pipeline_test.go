//go:build darwin && cgo

package video

import (
	"context"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"testing"
	"time"
	"yourdesk/internal/streampipeline"
)

// Same encoded GOP is decoded serially and through the receive handoff.
// Exact decoded-pixel hashes detect ordering, ownership and reference corruption.
func TestReceiveDoubleBufferNativeDecode(t *testing.T) {
	for _, codec := range []Codec{CodecSoftwareJPEG, CodecHardwareH264, CodecHardwareHEVC} {
		t.Run(string(codec), func(t *testing.T) {
			var encode func(image.Image, int) ([]byte, error)
			if codec == CodecSoftwareJPEG {
				e, _ := NewJPEGEncoderForCodec(codec)
				defer e.Close()
				encode = e.Encode
			} else {
				e, err := NewIntraEncoder(codec)
				if err != nil {
					t.Fatal(err)
				}
				defer e.Close()
				e.(GOPEncoder).SetKeyframeInterval(10)
				encode = e.Encode
			}
			packets := make([][]byte, 24)
			references := 0
			for i := range packets {
				src := image.NewRGBA(image.Rect(0, 0, 640, 360))
				draw.Draw(src, src.Bounds(), image.NewUniform(color.RGBA{30, 70, 100, 255}), image.Point{}, draw.Src)
				draw.Draw(src, image.Rect(i*10, 50, i*10+90, 150), image.NewUniform(color.RGBA{230, 180, 40, 255}), image.Point{}, draw.Src)
				var err error
				packets[i], err = encode(src, 80)
				if err != nil {
					t.Fatal(err)
				}
				if codec != CodecSoftwareJPEG {
					key, err := IsKeyframe(WireForCodec(codec), packets[i])
					if err != nil {
						t.Fatal(err)
					}
					if !key {
						references++
					}
				}
			}
			if codec != CodecSoftwareJPEG && references == 0 {
				t.Fatal("GOP has no reference frames")
			}
			decodeRun := func(parallel bool) ([]uint32, error) {
				var session DecodeSession
				defer session.Close()
				jpeg, _ := NewJPEGDecoderForCodec("software-jpeg")
				defer jpeg.Close()
				hashes := make([]uint32, 0, len(packets))
				var failure error
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				done := make(chan struct{})
				decode := func(index int) {
					var img image.Image
					var err error
					if codec == CodecSoftwareJPEG {
						img, err = jpeg.Decode(packets[index])
					} else {
						img, err = session.Decode(WireForCodec(codec), packets[index])
					}
					if err != nil {
						failure = err
					} else if img.Bounds().Dx() != 640 || img.Bounds().Dy() != 360 {
						failure = fmt.Errorf("unexpected bounds %v", img.Bounds())
					} else {
						rgba := image.NewRGBA(image.Rect(0, 0, 640, 360))
						draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
						hashes = append(hashes, crc32.ChecksumIEEE(rgba.Pix))
					}
					if index == len(packets)-1 {
						close(done)
					}
				}
				if parallel {
					c := streampipeline.NewConsumer(ctx, decode)
					for i := range packets {
						if !c.Submit(i) {
							c.Close()
							return nil, ctx.Err()
						}
					}
					select {
					case <-done:
					case <-ctx.Done():
						c.Close()
						return nil, ctx.Err()
					}
					c.Close()
				} else {
					for i := range packets {
						decode(i)
					}
				}
				return hashes, failure
			}
			serial, err := decodeRun(false)
			if err != nil {
				t.Fatal(err)
			}
			parallel, err := decodeRun(true)
			if err != nil {
				t.Fatal(err)
			}
			if len(serial) != 24 || len(parallel) != 24 {
				t.Fatalf("incomplete decode %d/%d", len(serial), len(parallel))
			}
			for i := range serial {
				if serial[i] != parallel[i] {
					t.Fatalf("frame %d pixel mismatch", i)
				}
			}
			t.Logf("24 frames matched, %d reference frames", references)
		})
	}
}
