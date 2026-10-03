package remoteaudio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
)

type captureTestDevice struct {
	processFunc func([]byte) ([]byte, error)
	isHardware  bool
	closed      int
}

func (d *captureTestDevice) process(data []byte) ([]byte, error) { return d.processFunc(data) }
func (d *captureTestDevice) hardware() bool                      { return d.isHardware }
func (d *captureTestDevice) close()                              { d.closed++ }

func captureTestData(size, seed int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(seed + i*31 + i/47)
	}
	return data
}

func TestSourceCaptureBufferPreservesPCM(t *testing.T) {
	for _, codec := range []string{"pcm", "aac", "opus"} {
		for _, fallback := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/fallback=%t", codec, fallback), func(t *testing.T) {
				settings := (Settings{Enabled: true, Generation: 1, Codec: codec}).Normalized()
				frameBytes := settings.FrameBytes()
				encodedData := []byte{7}
				if codec == "pcm" {
					encodedData = bytes.Repeat(encodedData, frameBytes)
				}
				var chunks, expected [][]byte
				var pending []byte
				// 包含空擷取、跨包殘餘、六包截斷及大於上限的擷取突波。
				for i, n := range []int{0, 1, frameBytes - 2, 3, frameBytes + 7, 3 * frameBytes, 6*frameBytes - 5, 0, 100*frameBytes + 9, frameBytes - 1, 1} {
					chunk := captureTestData(n, i)
					chunks = append(chunks, chunk)
					pending = append(pending, chunk...)
					if len(pending) > frameBytes*6 {
						pending = pending[len(pending)-frameBytes*6:]
					}
					for len(pending) >= frameBytes {
						expected = append(expected, bytes.Clone(pending[:frameBytes]))
						pending = pending[frameBytes:]
					}
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s := &Source{changed: make(chan struct{}, 1)}
				index, encoded, sent := 0, 0, 0
				capture := &captureTestDevice{processFunc: func([]byte) ([]byte, error) {
					if index == len(chunks) {
						cancel()
						return nil, nil
					}
					select {
					case s.changed <- struct{}{}:
					default:
					}
					data := chunks[index]
					index++
					return data, nil
				}}
				software := &captureTestDevice{processFunc: func(data []byte) ([]byte, error) {
					if encoded >= len(expected) || !bytes.Equal(data, expected[encoded]) {
						t.Fatalf("第 %d 包 PCM 內容或順序改變", encoded)
					}
					if cap(data) > frameBytes*6 {
						t.Fatal("擷取突波使 PCM 緩衝超出六包上限")
					}
					encoded++
					return encodedData, nil
				}}
				hardware := &captureTestDevice{isHardware: true, processFunc: func(data []byte) ([]byte, error) {
					if !bytes.Equal(data, expected[0]) {
						t.Fatal("硬體備援前 PCM 不正確")
					}
					return nil, errors.New("hardware unavailable")
				}}
				s.open = func(mode, codec, rate, pref int) (device, error) {
					if mode == 3 {
						return capture, nil
					}
					if fallback && pref == 1 {
						return hardware, nil
					}
					return software, nil
				}
				if _, err := s.Configure(settings); err != nil {
					t.Fatal(err)
				}
				s.Run(ctx, func(wire []byte) error {
					sent++
					packet, err := Parse(wire)
					if err != nil || packet.Sequence != uint64(sent) || packet.Generation != 1 || packet.Codec != settings.WireCodec() || !bytes.Equal(packet.Data, encodedData) {
						t.Fatalf("聲音封包內容或順序改變：%+v %v", packet, err)
					}
					return nil
				})
				if encoded != len(expected) || sent != encoded || capture.closed != 1 || software.closed != 1 || fallback && hardware.closed != 1 {
					t.Fatalf("遺失聲音影格或裝置未釋放：encoded=%d sent=%d", encoded, sent)
				}
			})
		}
	}
}

func TestSourceCaptureBufferClearedAfterEncoderFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Source{changed: make(chan struct{}, 1)}
	settings := (Settings{Enabled: true, Generation: 1, Codec: "pcm"}).Normalized()
	first := captureTestData(FrameBytes+13, 1)
	next := captureTestData(FrameBytes, 37)
	opens, reads, encodes, sends := 0, 0, 0, 0
	capture := &captureTestDevice{processFunc: func([]byte) ([]byte, error) {
		if ctx.Err() != nil {
			return nil, nil
		}
		reads++
		if reads == 1 {
			return first, nil
		}
		return next, nil
	}}
	encoder := &captureTestDevice{processFunc: func(data []byte) ([]byte, error) {
		encodes++
		if encodes == 1 {
			settings.Generation++
			if _, err := s.Configure(settings); err != nil {
				t.Fatal(err)
			}
			return nil, errors.New("encoder failure")
		}
		if !bytes.Equal(data, next) {
			t.Fatal("重新啟用後帶入上一世代的 PCM")
		}
		cancel()
		return data, nil
	}}
	s.open = func(mode, codec, rate, pref int) (device, error) {
		opens++
		if mode == 3 {
			return capture, nil
		}
		return encoder, nil
	}
	if _, err := s.Configure(settings); err != nil {
		t.Fatal(err)
	}
	s.Run(ctx, func(wire []byte) error {
		sends++
		p, err := Parse(wire)
		if err != nil || p.Generation != 2 || p.Sequence != 1 {
			t.Fatalf("重啟後封包錯誤：%+v %v", p, err)
		}
		return nil
	})
	if sends != 1 || encodes != 2 || opens != 4 || capture.closed != 2 || encoder.closed != 2 {
		t.Fatalf("錯誤恢復狀態不正確：sends=%d encodes=%d opens=%d", sends, encodes, opens)
	}
}

func BenchmarkAudioSourceCapture(b *testing.B) {
	for _, codec := range []string{"pcm", "opus"} {
		for _, kind := range []string{"frame", "partial", "burst"} {
			b.Run(codec+"/"+kind, func(b *testing.B) {
				settings := (Settings{Enabled: true, Generation: 1, Codec: codec}).Normalized()
				size, packets := settings.FrameBytes(), 1
				if kind == "partial" {
					size /= 2
				} else if kind == "burst" {
					size *= 100
					packets = 6
				}
				input, output := captureTestData(size, 1), []byte{7}
				if codec == "pcm" {
					output = bytes.Repeat(output, settings.FrameBytes())
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s := &Source{changed: make(chan struct{}, 1)}
				capture := &captureTestDevice{processFunc: func([]byte) ([]byte, error) {
					if ctx.Err() != nil {
						return nil, nil
					}
					select {
					case s.changed <- struct{}{}:
					default:
					}
					return input, nil
				}}
				encoder := &captureTestDevice{processFunc: func([]byte) ([]byte, error) { return output, nil }}
				s.open = func(mode, codec, rate, pref int) (device, error) {
					if mode == 3 {
						return capture, nil
					}
					return encoder, nil
				}
				if _, err := s.Configure(settings); err != nil {
					b.Fatal(err)
				}
				remaining := b.N * packets
				b.ReportAllocs()
				b.ResetTimer()
				s.Run(ctx, func([]byte) error {
					remaining--
					if remaining == 0 {
						cancel()
					}
					return nil
				})
				b.StopTimer()
				if remaining != 0 {
					b.Fatalf("未送出預期封包：remaining=%d", remaining)
				}
			})
		}
	}
}
