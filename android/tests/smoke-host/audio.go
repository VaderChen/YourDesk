package main

import (
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"time"
	"yourdesk/internal/p2p"
	"yourdesk/internal/remoteaudio"
)

//go:embed fixtures/*.bin
var audioFixtures embed.FS

func fixturePackets(name string) [][]byte {
	data, err := audioFixtures.ReadFile("fixtures/" + name)
	if err != nil {
		panic(err)
	}
	var packets [][]byte
	for len(data) > 0 {
		if len(data) < 4 {
			panic("音訊素材長度無效")
		}
		size := int(binary.BigEndian.Uint32(data))
		data = data[4:]
		if size <= 0 || size > len(data) {
			panic("音訊素材封包無效")
		}
		packets = append(packets, data[:size])
		data = data[size:]
	}
	return packets
}

// 僅使用合成音訊驗證真實 DataChannel／JNI／手機播放，不開啟主機錄音裝置。
func attachAudio(ctx context.Context, peer *p2p.Peer) {
	opus, aac := fixturePackets("audio-opus.bin"), fixturePackets("audio-aac.bin")
	var mu sync.Mutex
	var settings remoteaudio.Settings
	var lease time.Time
	mode := "all"
	for _, codec := range []string{"opus", "aac", "pcm", "fallback", "none"} {
		selected := codec
		_ = peer.RegisterCommand("smoke.audio."+codec, func(context.Context) (any, error) {
			mu.Lock()
			mode = selected
			mu.Unlock()
			return map[string]bool{"accepted": true}, nil
		})
	}
	_ = peer.RegisterCommand("audio.capabilities", func(context.Context) (any, error) {
		mu.Lock()
		selected := mode
		mu.Unlock()
		caps := []remoteaudio.Capability{}
		for _, codec := range []string{"opus", "aac", "pcm"} {
			if selected == "all" || selected == "fallback" || selected == codec {
				caps = append(caps, remoteaudio.Capability{Codec: codec, Encode: true, Decode: true})
			}
		}
		return caps, nil
	})
	_ = peer.RegisterCommandParams("audio.configure", func(_ context.Context, data json.RawMessage) (any, error) {
		var next remoteaudio.Settings
		if err := json.Unmarshal(data, &next); err != nil {
			return nil, err
		}
		if err := next.Validate(); err != nil {
			return nil, err
		}
		mu.Lock()
		defer mu.Unlock()
		if next.Generation == 0 || next.Generation < settings.Generation || (next.Generation == settings.Generation && next != settings) {
			return nil, errors.New("聲音設定世代無效")
		}
		settings = next
		lease = time.Now().Add(6 * time.Second)
		status := remoteaudio.Status{Generation: next.Generation, Enabled: next.Enabled, Codec: next.Codec, Bitrate: next.Bitrate()}
		if mode == "fallback" && next.Codec == "opus" && next.Enabled {
			status.Enabled = false
			status.Error = "合成測試：來源編碼器無法使用"
		}
		return status, nil
	})
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		var generation, sequence uint64
		var due time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-peer.Done():
				return
			case now := <-ticker.C:
				mu.Lock()
				current, until, selected := settings, lease, mode
				mu.Unlock()
				if !current.Enabled || now.After(until) || !peer.Connected() || (selected == "fallback" && current.Codec == "opus") {
					continue
				}
				if current.Generation != generation {
					generation = current.Generation
					sequence = 0
					due = now
				}
				if now.Before(due) {
					continue
				}
				var payload []byte
				samples := 1024
				switch current.WireCodec() {
				case 3:
					payload = opus[sequence%uint64(len(opus))]
					samples = 960
				case 1:
					payload = aac[sequence%uint64(len(aac))]
				case 2:
					payload = make([]byte, 4096)
					for i := 0; i < 1024; i++ {
						value := int16(1000 * math.Sin(2*math.Pi*440*float64(sequence*1024+uint64(i))/48000))
						binary.LittleEndian.PutUint16(payload[4*i:], uint16(value))
						binary.LittleEndian.PutUint16(payload[4*i+2:], uint16(value))
					}
				}
				sequence++
				_ = peer.SendAudio(remoteaudio.Packet{Generation: generation, Sequence: sequence, Codec: current.WireCodec(), Data: payload}.Marshal())
				due = due.Add(time.Duration(samples) * time.Second / 48000)
				if now.Sub(due) > 100*time.Millisecond {
					due = now
				}
			}
		}
	}()
}
