package remoteaudio

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"
)

type Status struct {
	Generation uint64 `json:"generation"`
	Enabled    bool   `json:"enabled"`
	Codec      string `json:"codec"`
	Bitrate    int    `json:"bitrate"`
	Hardware   bool   `json:"hardware"`
	Error      string `json:"error,omitempty"`
}

// Source 的租約須持續更新；關閉視窗、失去網路或停用通知遺失時仍會停止擷取。
type Source struct {
	changed chan struct{}
	open    func(int, int, int, int) (device, error)
	mu      sync.Mutex
	wanted  Settings
	lease   time.Time
	status  Status
}

func (s *Source) Configure(settings Settings) (Status, error) {
	settings = settings.Normalized()
	if err := settings.Validate(); err != nil {
		return Status{}, err
	}
	if settings.Codec == "auto" {
		return Status{}, errors.New("自動聲音編碼必須先完成兩端協商")
	}
	if settings.Generation == 0 {
		return Status{}, errors.New("聲音設定世代無效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if settings.Generation < s.wanted.Generation {
		return s.status, errors.New("聲音設定已過期")
	}
	if settings.Generation == s.wanted.Generation && settings != s.wanted {
		return s.status, errors.New("同一聲音世代不可更改設定")
	}
	if s.changed == nil {
		s.changed = make(chan struct{}, 1)
	}
	select {
	case s.changed <- struct{}{}:
	default:
	}
	s.wanted = settings
	s.lease = time.Now().Add(6 * time.Second)
	return s.status, nil
}
func (s *Source) snapshot() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.wanted
	if time.Now().After(s.lease) {
		v.Enabled = false
	}
	return v
}
func (s *Source) publish(status Status) { s.mu.Lock(); s.status = status; s.mu.Unlock() }
func (s *Source) Run(ctx context.Context, send func([]byte) error) {
	open := s.open
	if open == nil {
		open = openDevice
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var capture, encoder device
	var applied Settings
	var pending []byte
	var sequence uint64
	var emptyFrames int
	stop := func() {
		if capture != nil {
			capture.close()
			capture = nil
		}
		if encoder != nil {
			encoder.close()
			encoder = nil
		}
		pending = nil
		emptyFrames = 0
	}
	defer stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	ticker.Stop()
	s.mu.Lock()
	if s.changed == nil {
		s.changed = make(chan struct{}, 1)
	}
	changed := s.changed
	s.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-changed:
		}
		wanted := s.snapshot()
		if wanted != applied {
			stop()
			ticker.Stop()
			applied = wanted
			sequence = 0
			status := Status{Generation: wanted.Generation, Codec: wanted.Codec, Bitrate: wanted.Bitrate()}
			if wanted.Enabled {
				pref := 1
				if wanted.Codec == "aac-software" {
					pref = 0
				}
				var err error
				encoder, err = open(1, int(wanted.WireCodec()), wanted.Bitrate(), pref)
				if err == nil {
					capture, err = open(3, 0, 0, 0)
				}
				if err != nil {
					stop()
					status.Error = err.Error()
				} else {
					ticker.Reset(5 * time.Millisecond)
					status.Enabled = true
					status.Hardware = encoder.hardware()
				}
			}
			s.publish(status)
		}
		if capture == nil || encoder == nil {
			ticker.Stop()
			continue
		}
		data, err := capture.process(nil)
		if err != nil {
			stop()
			s.publish(Status{Generation: applied.Generation, Error: err.Error()})
			continue
		}
		pending = append(pending, data...)
		frameBytes := applied.FrameBytes()
		// 只保留最近六包 PCM，避免暫時阻塞後補播陳舊聲音。
		if len(pending) > frameBytes*6 {
			pending = pending[len(pending)-frameBytes*6:]
		}
		for len(pending) >= frameBytes {
			pcm := pending[:frameBytes]
			pending = pending[frameBytes:]
			encoded, e := encoder.process(pcm)
			if len(encoded) == 0 {
				emptyFrames++
			} else {
				emptyFrames = 0
			}
			if e == nil && emptyFrames > 20 {
				e = errors.New("聲音編碼器未產生輸出")
			}
			if e != nil && encoder.hardware() {
				encoder.close()
				emptyFrames = 0
				encoder, e = open(1, int(applied.WireCodec()), applied.Bitrate(), 0)
				if e == nil {
					encoded, e = encoder.process(pcm)
					s.publish(Status{Generation: applied.Generation, Enabled: true, Codec: applied.Codec, Bitrate: applied.Bitrate()})
				}
			}
			if e != nil {
				stop()
				s.publish(Status{Generation: applied.Generation, Error: e.Error()})
				break
			}
			if len(encoded) > 0 && len(encoded) <= MaxPacket {
				sequence++
				_ = send(Packet{applied.Generation, sequence, applied.WireCodec(), encoded}.Marshal())
			}
		}
	}
}

// RunReceiver 只接受目前啟用世代；先開原生解碼器、後開播放裝置，不觸碰麥克風。
func RunReceiver(ctx context.Context, inbox <-chan []byte, settings func() Settings, report func(string), statusChanged ...func(Status)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var decoder, playback device
	var current Settings
	var sequence uint64
	var lastPacket time.Time
	var failed bool
	var lastStatus Status
	publish := func(status Status) {
		if status == lastStatus {
			return
		}
		lastStatus = status
		for _, changed := range statusChanged {
			if changed != nil {
				changed(status)
			}
		}
	}
	stop := func() {
		if decoder != nil {
			decoder.close()
			decoder = nil
		}
		if playback != nil {
			playback.close()
			playback = nil
		}
		sequence = 0
		publish(Status{Generation: current.Generation})
	}
	defer stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var wire []byte
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case wire = <-inbox:
		}
		wanted := settings()
		if wanted != current {
			stop()
			current = wanted
			failed = false
			publish(Status{Generation: current.Generation})
		}
		if !current.Enabled || failed {
			continue
		}
		if len(wire) == 0 {
			if playback != nil && time.Since(lastPacket) > 500*time.Millisecond {
				stop()
			}
			continue
		}
		p, err := Parse(wire)
		if err != nil || p.Generation != current.Generation || p.Codec != current.WireCodec() || p.Sequence <= sequence {
			continue
		}
		// 缺包後重建解碼／播放，避免 AAC 濾波歷史或裝置佇列殘留。
		if sequence > 0 && p.Sequence > sequence+1 {
			stop()
		}
		sequence = p.Sequence
		lastPacket = time.Now()
		if decoder == nil {
			decoder, err = openDevice(2, int(p.Codec), current.Bitrate(), 1)
		}
		var pcm []byte
		if err == nil {
			pcm, err = decoder.process(p.Data)
		}
		if err != nil && decoder != nil && decoder.hardware() {
			decoder.close()
			decoder, err = openNative(2, int(p.Codec), current.Bitrate(), 0)
			if err == nil {
				pcm, err = decoder.process(p.Data)
			}
		}
		if err == nil && len(pcm) > 0 {
			if playback == nil {
				playback, err = openNative(4, 0, 0, 0)
			}
			if err == nil {
				_, err = playback.process(pcm)
			}
		}
		if err != nil {
			stop()
			failed = true
			publish(Status{Generation: current.Generation, Error: err.Error()})
			report(err.Error())
		} else if decoder != nil {
			publish(Status{Generation: current.Generation, Enabled: true, Codec: current.Codec, Bitrate: current.Bitrate(), Hardware: decoder.hardware()})
		}
	}
}
