package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// 音訊 API 由 Android 背景工作者呼叫；不在 UI 或 P2P 接收回呼等待遠端。
func (v *Viewer) audioCommand(method string, params any) (string, error) {
	v.mu.Lock()
	p, epoch := v.peer, v.generation
	v.mu.Unlock()
	if p == nil {
		return "", fmt.Errorf("遠端尚未連線")
	}
	var data []byte
	if params != nil {
		data, _ = json.Marshal(params)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reply, err := p.CallCommandParams(ctx, method, data)
	if err != nil {
		return "", err
	}
	v.mu.Lock()
	current := v.peer == p && v.generation == epoch
	v.mu.Unlock()
	if !current {
		return "", fmt.Errorf("聲音工作階段已結束")
	}
	return string(reply.Result), nil
}

func (v *Viewer) AudioCapabilitiesJSON() (string, error) {
	return v.audioCommand("audio.capabilities", nil)
}

func (v *Viewer) ConfigureAudio(enabled bool, codec, profile string, generation int64) (string, error) {
	if generation <= 0 || (codec != "opus" && codec != "aac" && codec != "aac-software" && codec != "pcm") ||
		(profile != "fast" && profile != "standard" && profile != "high") {
		return "", fmt.Errorf("聲音設定無效")
	}
	return v.audioCommand("audio.configure", struct {
		Enabled    bool   `json:"enabled"`
		Codec      string `json:"codec"`
		Profile    string `json:"profile"`
		Generation int64  `json:"generation"`
	}{enabled, codec, profile, generation})
}

// ReadAudioPacket 不阻塞。每包包含 YDA1 世代與序號，由原生播放端再次驗證。
func (v *Viewer) ReadAudioPacket() []byte {
	v.mu.Lock()
	p := v.peer
	v.mu.Unlock()
	if p == nil || !p.Connected() {
		return nil
	}
	return p.ReadAudioPacket()
}
