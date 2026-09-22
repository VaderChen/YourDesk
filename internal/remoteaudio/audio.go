// Package remoteaudio 傳送系統輸出聲音，不擷取麥克風。所有原生工作由固定執行緒呼叫。
package remoteaudio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"sync"
)

const SampleRate = 48000
const Channels = 2
const FrameBytes = 1024 * Channels * 2
const MaxPacket = 8192
const OpusFrameSamples = 960
const MaxOpusPacket = 1275

type Settings struct {
	Enabled    bool   `json:"enabled"`
	Codec      string `json:"codec"`
	Profile    string `json:"profile"`
	Generation uint64 `json:"generation"`
}

func (s Settings) Normalized() Settings {
	if s.Codec == "" {
		s.Codec = "opus"
	}
	if s.Profile == "" {
		s.Profile = "standard"
	}
	return s
}
func (s Settings) Validate() error {
	if s.Codec != "auto" && s.Codec != "opus" && s.Codec != "aac" && s.Codec != "aac-software" && s.Codec != "pcm" {
		return errors.New("不支援的聲音編碼")
	}
	if s.Profile != "fast" && s.Profile != "standard" && s.Profile != "high" {
		return errors.New("不支援的聲音品質")
	}
	return nil
}
func (s Settings) Bitrate() int {
	if s.Codec == "pcm" {
		return SampleRate * Channels * 16
	}
	if s.Codec == "opus" {
		switch s.Profile {
		case "fast":
			return 48000
		case "high":
			return 160000
		default:
			return 96000
		}
	}
	switch s.Profile {
	case "fast":
		return 96000
	case "high":
		return 192000
	default:
		return 128000
	}
}
func (s Settings) FrameBytes() int {
	if s.Codec == "opus" {
		return OpusFrameSamples * Channels * 2
	}
	return FrameBytes
}
func (s Settings) WireCodec() byte {
	if s.Codec == "opus" {
		return 3
	}
	if s.Codec == "pcm" {
		return 2
	}
	return 1
}

type Capability struct {
	Codec          string `json:"codec"`
	Encode         bool   `json:"encode"`
	Decode         bool   `json:"decode"`
	HardwareEncode bool   `json:"hardwareEncode"`
	HardwareDecode bool   `json:"hardwareDecode"`
}

// Probe 不開啟錄音或播放裝置；建立編解碼器並以合成 PCM 實際往返驗證。
func Probe() []Capability {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	out := []Capability{}
	for _, codec := range []string{"aac", "opus"} {
		settings := (Settings{Codec: codec}).Normalized()
		cap := Capability{Codec: codec}
		open := openNative
		if codec == "opus" {
			open = func(mode, _ int, bitrate, preference int) (device, error) { return openOpus(mode, bitrate, preference) }
		}
		for _, preference := range []int{0, 2} {
			enc, err := open(1, int(settings.WireCodec()), settings.Bitrate(), preference)
			if err != nil {
				continue
			}
			pcm := make([]byte, settings.FrameBytes())
			// 避免僅靠靜音封包判定可用。
			for i := 0; i < len(pcm)/4; i++ {
				v := int16((i%80 - 40) * 300)
				binary.LittleEndian.PutUint16(pcm[i*4:], uint16(v))
				binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(v))
			}
			var packets [][]byte
			for i := 0; i < 16; i++ {
				packet, e := enc.process(pcm)
				if e != nil {
					err = e
					break
				}
				if len(packet) > 0 {
					packets = append(packets, packet)
				}
			}
			enc.close()
			if err != nil || len(packets) == 0 {
				continue
			}
			cap.Encode = true
			if preference == 2 {
				cap.HardwareEncode = true
			}
			for _, decoderPreference := range []int{0, 2} {
				dec, e := open(2, int(settings.WireCodec()), settings.Bitrate(), decoderPreference)
				if e != nil {
					continue
				}
				var energy int64
				for _, packet := range packets {
					decoded, x := dec.process(packet)
					if x != nil {
						e = x
						break
					}
					for i := 0; i+1 < len(decoded); i += 2 {
						v := int64(int16(binary.LittleEndian.Uint16(decoded[i:])))
						energy += v * v
					}
				}
				dec.close()
				if e == nil && energy > 100000 {
					cap.Decode = true
					if decoderPreference == 2 {
						cap.HardwareDecode = true
					}
				}
			}
		}
		out = append(out, cap)
	}
	out = append(out, Capability{Codec: "pcm", Encode: platformSupported(), Decode: platformSupported()})
	return out
}

type Packet struct {
	Generation, Sequence uint64
	Codec                byte
	Data                 []byte
}

func (p Packet) Marshal() []byte {
	b := make([]byte, 24+len(p.Data))
	copy(b, "YDA1")
	b[4] = p.Codec
	binary.BigEndian.PutUint64(b[8:], p.Generation)
	binary.BigEndian.PutUint64(b[16:], p.Sequence)
	copy(b[24:], p.Data)
	return b
}
func Parse(b []byte) (Packet, error) {
	if len(b) <= 24 || len(b) > MaxPacket+24 || string(b[:4]) != "YDA1" || b[5] != 0 || b[6] != 0 || b[7] != 0 {
		return Packet{}, errors.New("聲音封包無效")
	}
	p := Packet{binary.BigEndian.Uint64(b[8:]), binary.BigEndian.Uint64(b[16:]), b[4], b[24:]}
	if (p.Codec != 1 && p.Codec != 2 && p.Codec != 3) || p.Generation == 0 || p.Sequence == 0 || (p.Codec == 2 && len(p.Data) != FrameBytes) || (p.Codec == 3 && len(p.Data) > MaxOpusPacket) {
		return Packet{}, errors.New("聲音封包格式無效")
	}
	return p, nil
}

// native 只可在擁有它的固定 OS 執行緒使用與關閉。
type device interface {
	process([]byte) ([]byte, error)
	close()
	hardware() bool
}

func nativeError(code int) error { return fmt.Errorf("原生聲音處理失敗 (%d)", code) }

var capabilityOnce sync.Once
var capabilities []Capability

// CachedCapabilities 僅以合成資料探測，供自動協商及既有硬體備援共用。
func CachedCapabilities() []Capability {
	capabilityOnce.Do(func() { capabilities = Probe() })
	return append([]Capability(nil), capabilities...)
}

// AutomaticCodec 依選單順序比對來源編碼與接收端解碼；AAC 第一順位需來源實測硬體可用。
func AutomaticCodec(source, receiver []Capability) (string, error) {
	for _, codec := range []string{"aac", "opus", "aac-software", "pcm"} {
		wire := codec
		if wire == "aac-software" {
			wire = "aac"
		}
		encode, decode := false, false
		for _, cap := range source {
			if cap.Codec == wire {
				encode = cap.Encode && (codec != "aac" || cap.HardwareEncode)
			}
		}
		for _, cap := range receiver {
			if cap.Codec == wire {
				decode = cap.Decode
			}
		}
		if encode && decode {
			return codec, nil
		}
	}
	return "", errors.New("兩端沒有共同可用的聲音編碼")
}

// 每個程序只驗證一次；不以硬體宣告或建立成功冒充實際可用。
func openDevice(mode, codec, bitrate, preference int) (device, error) {
	if codec == 3 && (mode == 1 || mode == 2) {
		return openOpus(mode, bitrate, preference)
	}
	if codec == 1 && (mode == 1 || mode == 2) && preference == 1 {
		capabilityOnce.Do(func() { capabilities = Probe() })
		preference = 0
		for _, cap := range capabilities {
			if cap.Codec == "aac" && ((mode == 1 && cap.HardwareEncode) || (mode == 2 && cap.HardwareDecode)) {
				preference = 2
			}
		}
		d, err := openNative(mode, codec, bitrate, preference)
		if err != nil && preference == 2 {
			return openNative(mode, codec, bitrate, 0)
		}
		return d, err
	}
	return openNative(mode, codec, bitrate, preference)
}
