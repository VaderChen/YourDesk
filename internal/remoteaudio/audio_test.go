package remoteaudio

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestProfilesAndPackets(t *testing.T) {
	defaults := (Settings{}).Normalized()
	if defaults.Enabled || defaults.Codec != "opus" || defaults.Profile != "standard" || defaults.FrameBytes() != 3840 {
		t.Fatal(defaults)
	}
	for i, profile := range []string{"fast", "standard", "high"} {
		settings := (Settings{Profile: profile}).Normalized()
		if settings.Validate() != nil || settings.Bitrate() != []int{48000, 96000, 160000}[i] || settings.WireCodec() != 3 {
			t.Fatal(settings)
		}
	}
	for _, codec := range []string{"aac", "aac-software", "pcm"} {
		settings := (Settings{Codec: codec}).Normalized()
		if settings.Codec != codec || settings.FrameBytes() != 4096 {
			t.Fatal("不可覆寫既有格式", settings)
		}
	}
	if _, err := Parse((Packet{1, 1, 3, make([]byte, MaxOpusPacket+1)}).Marshal()); err == nil {
		t.Fatal("接受過大 Opus 封包")
	}

	for i, p := range []string{"fast", "standard", "high"} {
		s := Settings{Codec: "aac", Profile: p}
		if s.Validate() != nil || s.Bitrate() != []int{96000, 128000, 192000}[i] {
			t.Fatal(s)
		}
	}
	p := Packet{1, 2, 2, make([]byte, FrameBytes)}
	wire := p.Marshal()
	q, e := Parse(wire)
	if e != nil || q.Sequence != 2 || q.Generation != 1 {
		t.Fatal(q, e)
	}
	for _, b := range [][]byte{nil, wire[:20], wire[:len(wire)-1], make([]byte, MaxPacket+25)} {
		if _, e := Parse(b); e == nil {
			t.Fatal("接受無效封包")
		}
	}
	binary.BigEndian.PutUint64(wire[8:], 0)
	if _, e := Parse(wire); e == nil {
		t.Fatal("接受零世代")
	}
}
func TestNativeAACRoundTrip(t *testing.T) {
	if !platformSupported() {
		t.Skip("無原生聲音")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	caps := Probe()
	t.Logf("原生音訊能力: %+v", caps)
	if !caps[0].Encode || !caps[0].Decode {
		t.Fatal("AAC 往返失敗")
	}
	for _, bitrate := range []int{96000, 128000, 192000} {
		e, err := openNative(1, 1, bitrate, 1)
		if err != nil {
			t.Fatal(err)
		}
		e.close()
	}
}

func TestNativeAACContinuous(t *testing.T) {
	if !platformSupported() {
		t.Skip("無原生聲音")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, rate := range []int{96000, 128000, 192000} {
		enc, err := openNative(1, 1, rate, 1)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := openNative(2, 1, rate, 1)
		if err != nil {
			enc.close()
			t.Fatal(err)
		}
		var energy int64
		var bytes, packets int
		for n := 0; n < 100; n++ {
			pcm := make([]byte, FrameBytes)
			for i := 0; i < 1024; i++ {
				v := int16(6000 * math.Sin(2*math.Pi*440*float64(n*1024+i)/48000))
				binary.LittleEndian.PutUint16(pcm[i*4:], uint16(v))
				binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(v))
			}
			packet, e := enc.process(pcm)
			if e != nil {
				t.Fatal(e)
			}
			if len(packet) == 0 {
				continue
			}
			packets++
			decoded, e := dec.process(packet)
			if e != nil {
				t.Fatal(e)
			}
			bytes += len(decoded)
			for i := 0; i+1 < len(decoded); i += 2 {
				v := int64(int16(binary.LittleEndian.Uint16(decoded[i:])))
				energy += v * v
			}
		}
		enc.close()
		dec.close()
		if packets < 90 || bytes < FrameBytes*85 || energy/int64(bytes/2) < 1000000 {
			t.Fatalf("%d bps 輸出不足或靜音：%d packets %d bytes %d energy", rate, packets, bytes, energy)
		}
	}
}

// 明確指定環境變數才開系統擷取；只統計合成測試聲音，不儲存錄音。
func TestNativeSystemCaptureSmoke(t *testing.T) {
	if os.Getenv("YOURDESK_AUDIO_SMOKE") != "1" || runtime.GOOS != "darwin" {
		t.Skip("需明確啟用裝置 Smoke")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	capture, err := openNative(3, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer capture.close()
	// 播放程序與擷取程序分離，驗證排除自身播放的迴音保護。
	player := exec.Command("/usr/bin/afplay", "/System/Library/Sounds/Glass.aiff")
	if err = player.Start(); err != nil {
		t.Fatal(err)
	}
	defer player.Wait()
	var energy int64
	var count int
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		b, e := capture.process(nil)
		if e != nil {
			t.Fatal(e)
		}
		count += len(b)
		for i := 0; i+1 < len(b); i += 2 {
			v := int64(int16(binary.LittleEndian.Uint16(b[i:])))
			energy += v * v
		}
		time.Sleep(5 * time.Millisecond)
	}
	if count == 0 || energy == 0 {
		t.Fatalf("未擷取到測試聲音：%d bytes energy=%d", count, energy)
	}
	t.Logf("系統聲音擷取成功：%d bytes（未儲存）", count)
}

func TestNativePlaybackSmoke(t *testing.T) {
	if os.Getenv("YOURDESK_AUDIO_SMOKE") != "1" {
		t.Skip("需明確啟用裝置 Smoke")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	playback, err := openNative(4, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer playback.close()
	pcm := make([]byte, FrameBytes)
	for i := 0; i < 1024; i++ {
		v := int16(1000 * math.Sin(2*math.Pi*440*float64(i)/48000))
		binary.LittleEndian.PutUint16(pcm[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(v))
	}
	for n := 0; n < 10; n++ {
		if _, err = playback.process(pcm); err != nil {
			t.Fatal(err)
		}
		time.Sleep(22 * time.Millisecond)
	}
}

func TestNativeSessionSmoke(t *testing.T) {
	if os.Getenv("YOURDESK_AUDIO_SMOKE") != "1" || runtime.GOOS != "darwin" {
		t.Skip("需明確啟用裝置 Smoke")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &Source{}
	inbox := make(chan []byte, 8)
	var packets atomic.Int32
	var settings atomic.Pointer[Settings]
	current := Settings{Enabled: true, Codec: "aac", Profile: "standard", Generation: 1}
	settings.Store(&current)
	doneSource, doneSink := make(chan struct{}), make(chan struct{})
	failures := make(chan string, 8)
	go func() {
		defer close(doneSource)
		source.Run(ctx, func(b []byte) error {
			packets.Add(1)
			select {
			case inbox <- b:
			default:
			}
			return nil
		})
	}()
	go func() {
		defer close(doneSink)
		RunReceiver(ctx, inbox, func() Settings { return *settings.Load() }, func(e string) {
			select {
			case failures <- e:
			default:
			}
		})
	}()
	defer func() {
		cancel()
		select {
		case <-doneSource:
		case <-time.After(5 * time.Second):
			t.Error("擷取未結束")
		}
		select {
		case <-doneSink:
		case <-time.After(5 * time.Second):
			t.Error("播放未結束")
		}
	}()
	trials := []Settings{{Codec: "aac", Profile: "standard"}, {Codec: "pcm", Profile: "standard"}}
	if opusAvailable {
		trials = append([]Settings{{Codec: "opus", Profile: "standard"}, {Codec: "opus", Profile: "fast"}, {Codec: "opus", Profile: "high"}}, trials...)
	}
	for i, trial := range trials {
		s := current
		s.Profile = trial.Profile
		s.Codec = trial.Codec
		s.Generation = uint64(i + 1)
		settings.Store(&s)
		if _, err := source.Configure(s); err != nil {
			t.Fatal(err)
		}
		baseline := packets.Load()
		deadline := time.Now().Add(5 * time.Second)
		for {
			status, _ := source.Configure(s)
			if status.Error != "" {
				t.Fatal(status.Error)
			}
			if status.Enabled && status.Generation == s.Generation {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("聲音來源未就緒")
			}
			time.Sleep(25 * time.Millisecond)
		}
		if err := exec.Command("/usr/bin/afplay", "/System/Library/Sounds/Glass.aiff").Run(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
		if packets.Load()-baseline < 10 {
			t.Fatalf("%s/%s 未持續送出音訊", s.Codec, s.Profile)
		}
	}
	off := Settings{Codec: "opus", Profile: "high", Generation: uint64(len(trials) + 1)}
	settings.Store(&off)
	source.Configure(off)
	time.Sleep(150 * time.Millisecond)
	baseline := packets.Load()
	time.Sleep(150 * time.Millisecond)
	if packets.Load() != baseline {
		t.Fatal("關閉後仍傳送聲音")
	}
	select {
	case err := <-failures:
		t.Fatal(err)
	default:
	}
	t.Logf("擷取 → 編碼 → 有界佇列 → 解碼 → 播放：%d 包；品質／編碼切換及關閉成功", packets.Load())
}
