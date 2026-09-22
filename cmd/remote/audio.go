package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
	"yourdesk/internal/remoteaudio"
)

var viewerAudio atomic.Pointer[remoteaudio.Settings]

type audioDisplayStatus struct {
	Enabled  bool               `json:"enabled"`
	Error    string             `json:"error,omitempty"`
	Source   remoteaudio.Status `json:"source"`
	Receiver remoteaudio.Status `json:"receiver"`
}

// 控制協商與聲音解碼均在背景，UI 更新不等待硬體或網路。
func (c *viewerConnection) startAudio(ctx context.Context, g *game) {
	var active atomic.Pointer[remoteaudio.Settings]
	var failed atomic.Bool
	var receiverStatus atomic.Pointer[remoteaudio.Status]
	var receiverError atomic.Pointer[string]
	desired := func() remoteaudio.Settings {
		s := remoteaudio.Settings{Codec: "opus", Profile: "standard"}
		if p := viewerAudio.Load(); p != nil {
			s = *p
		}
		quality := int(g.audioQuality.Load())
		if quality >= 0 && quality < 3 {
			s.Profile = []string{"fast", "standard", "high"}[quality]
		}
		return s.Normalized()
	}
	report := func(message string) {
		slog.Warn("遠端聲音", "error", message)
		nativeShowInputNotice("遠端聲音：" + message)
		emitUIEvent("audio-error", "遠端聲音："+message)
	}
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		remoteaudio.RunReceiver(ctx, c.peer.AudioMessages(), func() remoteaudio.Settings {
			s := desired()
			if p := active.Load(); p != nil && (s.Codec == "auto" || p.Codec == s.Codec) && p.Profile == s.Profile {
				enabled := s.Enabled && p.Enabled && !failed.Load()
				s = *p
				s.Enabled = enabled
			} else {
				s.Enabled = false
			}
			return s
		}, func(message string) { receiverError.Store(&message); failed.Store(true); report(message) }, func(status remoteaudio.Status) { receiverStatus.Store(&status) })
	}()
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		defer nativeSetAudioStatus(audioDisplayStatus{})
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		var previous remoteaudio.Settings
		var generation uint64
		var sent time.Time
		var lastError, lastBackend string
		var automaticCodec string
		var automaticTried bool
		var sourceStatus remoteaudio.Status
		var displayed audioDisplayStatus
		publish := func(s remoteaudio.Settings) {
			value := audioDisplayStatus{Enabled: desired().Enabled, Error: lastError}
			if sourceStatus.Generation == s.Generation {
				value.Source = sourceStatus
			}
			if receiver := receiverStatus.Load(); receiver != nil {
				if receiver.Generation == s.Generation {
					value.Receiver = *receiver
				}
			}
			if message := receiverError.Load(); failed.Load() && message != nil {
				value.Error = *message
			}
			if value != displayed && ctx.Err() == nil {
				displayed = value
				nativeSetAudioStatus(value)
			}
		}
		var muted bool
		started := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.peer.Done():
				return
			case <-ticker.C:
			}
			s := desired()
			if s != previous {
				failed.Store(false)
				receiverError.Store(nil)
				previous = s
				generation++
				sent = time.Time{}
				lastError = ""
				automaticCodec, automaticTried = "", false
			}
			if failed.Load() && s.Enabled {
				s.Enabled = false
				if !muted {
					generation++
					muted = true
					sent = time.Time{}
				}
			} else {
				muted = false
			}
			if s.Codec == "auto" {
				if s.Enabled && !automaticTried && (c.peer.SupportsCommand("audio.capabilities") || time.Since(started) > 5*time.Second) {
					automaticTried = true
					var err error
					if !c.peer.SupportsCommand("audio.capabilities") {
						err = fmt.Errorf("對方尚未支援自動聲音編碼，請更新對方的 YourDesk。")
					} else {
						var source []remoteaudio.Capability
						reply, callErr := c.peer.CallCommand(ctx, "audio.capabilities")
						err = callErr
						if err == nil {
							err = json.Unmarshal(reply.Result, &source)
						}
						if err == nil {
							automaticCodec, err = remoteaudio.AutomaticCodec(source, remoteaudio.CachedCapabilities())
						}
					}
					if err != nil && ctx.Err() == nil {
						lastError = err.Error()
						report(lastError)
					}
					if automaticCodec != "" {
						// 等待協商時可能已送出停用租約；新格式必須使用新的世代。
						generation++
						sent = time.Time{}
					}
				}
				s.Codec = automaticCodec
				if s.Codec == "" {
					s.Codec = "opus"
					s.Enabled = false
				}
			}
			s.Generation = generation
			active.Store(&s)
			publish(s)
			if !c.peer.SupportsCommand("audio.configure") {
				if s.Enabled && time.Since(started) > 5*time.Second && lastError == "" {
					lastError = "對方尚未支援遠端聲音，請更新對方的 YourDesk。"
					report(lastError)
					publish(s)
				}
				continue
			}
			if (!s.Enabled && !sent.IsZero()) || time.Since(sent) < time.Second {
				continue
			}
			sent = time.Now()
			data, _ := json.Marshal(s)
			reply, err := c.peer.CallCommandParams(ctx, "audio.configure", data)
			if err != nil {
				if ctx.Err() == nil && s.Enabled && lastError != err.Error() {
					lastError = err.Error()
					report(lastError)
				}
				continue
			}
			var status remoteaudio.Status
			if json.Unmarshal(reply.Result, &status) != nil || status.Generation != s.Generation {
				continue
			}
			if status.Error != "" && status.Error != lastError {
				lastError = status.Error
				report(status.Error)
			}
			sourceStatus = status
			if status.Enabled {
				lastError = ""
				backend := fmt.Sprintf("%s/%d/hardware=%t", status.Codec, status.Bitrate, status.Hardware)
				if backend != lastBackend {
					slog.Info("遠端聲音實際編碼", "backend", backend)
					lastBackend = backend
				}
			}
			publish(s)
		}
	}()
}
