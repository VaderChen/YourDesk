package remoteaudio

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"testing"
)

func audioTestPCM(size int) []byte {
	pcm := make([]byte, size)
	for i := 0; i < len(pcm)/2; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16((i%80-40)*300)))
	}
	return pcm
}

// Retained packets and decoded samples must survive later calls and close.
func TestAudioOutputOwnership(t *testing.T) {
	for _, codec := range []string{"pcm", "aac-software", "opus"} {
		t.Run(codec, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			if codec == "opus" && !opusAvailable || codec != "opus" && !platformSupported() {
				t.Skip("codec unavailable on this build")
			}
			s := (Settings{Codec: codec}).Normalized()
			enc, err := openDevice(1, int(s.WireCodec()), s.Bitrate(), 0)
			if err != nil {
				t.Fatal(err)
			}
			defer enc.close()
			dec, err := openDevice(2, int(s.WireCodec()), s.Bitrate(), 0)
			if err != nil {
				t.Fatal(err)
			}
			defer dec.close()
			pcm := audioTestPCM(s.FrameBytes())
			var outputs, expected [][]byte
			for i := 0; i < 32; i++ {
				packet, err := enc.process(pcm)
				if err != nil {
					t.Fatal(err)
				}
				if len(packet) == 0 {
					continue
				}
				outputs = append(outputs, packet)
				expected = append(expected, bytes.Clone(packet))
				decoded, err := dec.process(packet)
				if err != nil {
					t.Fatal(err)
				}
				outputs = append(outputs, decoded)
				expected = append(expected, bytes.Clone(decoded))
				pcm[i%len(pcm)] ^= byte(i + 1)
			}
			enc.close()
			dec.close()
			if len(outputs) == 0 {
				t.Fatal("codec produced no output")
			}
			for i := range outputs {
				if !bytes.Equal(outputs[i], expected[i]) {
					t.Fatalf("output %d changed after subsequent processing or close", i)
				}
			}
		})
	}
}

func BenchmarkAudioProcess(b *testing.B) {
	for _, codec := range []string{"pcm", "aac-software", "opus"} {
		for _, mode := range []int{1, 2} {
			direction := "encode"
			if mode == 2 {
				direction = "decode"
			}
			b.Run(codec+"/"+direction, func(b *testing.B) {
				if codec == "opus" && !opusAvailable || codec != "opus" && !platformSupported() {
					b.Skip("codec unavailable on this build")
				}
				runtime.LockOSThread()
				defer runtime.UnlockOSThread()
				s := (Settings{Codec: codec}).Normalized()
				d, err := openDevice(mode, int(s.WireCodec()), s.Bitrate(), 0)
				if err != nil {
					b.Fatal(err)
				}
				defer d.close()
				input := audioTestPCM(s.FrameBytes())
				if mode == 2 {
					enc, err := openDevice(1, int(s.WireCodec()), s.Bitrate(), 0)
					if err != nil {
						b.Fatal(err)
					}
					defer enc.close()
					pcm := input
					for i := 0; i < 16; i++ {
						input, err = enc.process(pcm)
						if err != nil {
							b.Fatal(err)
						}
						if len(input) > 0 {
							break
						}
					}
					if len(input) == 0 {
						b.Fatal("encoder produced no packet")
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := d.process(input); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// PCM passthrough exercises a native zero-output call without opening a
// capture/playback device or requiring audio permissions.
func BenchmarkNativeAudioEmpty(b *testing.B) {
	if !platformSupported() {
		b.Skip("native audio unavailable on this build")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	d, err := openNative(1, 2, 0, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer d.close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := d.process(nil)
		if err != nil || len(out) != 0 {
			b.Fatalf("expected empty output: %d bytes, %v", len(out), err)
		}
	}
}
