//go:build cgo && opus

package remoteaudio

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestOpusContinuous(t *testing.T) {
	for _, profile := range []string{"fast", "standard", "high"} {
		t.Run(profile, func(t *testing.T) {
			s := (Settings{Profile: profile}).Normalized()
			enc, err := openDevice(1, int(s.WireCodec()), s.Bitrate(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer enc.close()
			dec, err := openDevice(2, int(s.WireCodec()), s.Bitrate(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer dec.close()
			if enc.hardware() || dec.hardware() {
				t.Fatal("軟體 Opus 不可宣稱硬體加速")
			}
			var energy [2]int64
			var total int
			for frame := 0; frame < 100; frame++ {
				pcm := make([]byte, s.FrameBytes())
				for i := 0; i < OpusFrameSamples; i++ {
					for ch, hz := range []float64{440, 880} {
						v := int16(6000 * math.Sin(2*math.Pi*hz*float64(frame*OpusFrameSamples+i)/SampleRate))
						binary.LittleEndian.PutUint16(pcm[(i*Channels+ch)*2:], uint16(v))
					}
				}
				packet, err := enc.process(pcm)
				if err != nil {
					t.Fatal(err)
				}
				total += len(packet)
				wire, err := Parse((Packet{1, uint64(frame + 1), s.WireCodec(), packet}).Marshal())
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := dec.process(wire.Data)
				if err != nil || len(decoded) != s.FrameBytes() {
					t.Fatalf("輸出須為 20 ms：%d %v", len(decoded), err)
				}
				for i := 0; i < len(decoded)/2; i++ {
					v := int64(int16(binary.LittleEndian.Uint16(decoded[i*2:])))
					energy[i%Channels] += v * v
				}
			}
			for _, e := range energy {
				if e/(100*OpusFrameSamples) < 1000000 {
					t.Fatal("解碼聲道靜音或音量異常")
				}
			}
			if total*8 > s.Bitrate()*3 {
				t.Fatalf("兩秒音訊流量異常：%d bytes", total)
			}
			t.Logf("100 包／2 秒；實際平均 %d bps", total*4)
			for _, bad := range [][]byte{nil, {0x80}, make([]byte, MaxOpusPacket+1)} {
				if _, err := dec.process(bad); err == nil {
					t.Fatal("未拒絕無效／非 20 ms 封包")
				}
			}
			if _, err := enc.process(make([]byte, FrameBytes)); err == nil {
				t.Fatal("誤用 AAC 框長仍成功")
			}
		})
	}
}

func TestOpusProbe(t *testing.T) {
	for _, cap := range Probe() {
		if cap.Codec == "opus" {
			if !cap.Encode || !cap.Decode || cap.HardwareEncode || cap.HardwareDecode {
				t.Fatal(cap)
			}
			return
		}
	}
	t.Fatal("能力未包含 Opus")
}
