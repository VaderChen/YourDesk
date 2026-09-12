package optimization

import (
	"runtime"
	"testing"
)

func TestDecoderPolicyValidation(t *testing.T) {
	old := current.Load()
	defer current.Store(old)
	p := Policy{Version: 1, OS: runtime.GOOS, Arch: runtime.GOARCH, Decoders: []Decoder{{Codec: "h264", Width: 128, Height: 128, Mode: "hardware"}}}
	if !Apply(p) {
		t.Fatal("有效解碼策略被拒絕")
	}
	p.Decoders[0].Mode = "unavailable"
	s := Snapshot()
	if s.DecoderDecision("h264", 128, 128) != "hardware" {
		t.Fatal("輸入修改污染快照")
	}
	s.Decoders[0].Mode = "software"
	if Snapshot().DecoderDecision("h264", 128, 128) != "hardware" {
		t.Fatal("輸出修改污染快照")
	}
	if s.DecoderDecision("h264", 1920, 1080) != "" {
		t.Fatal("錯誤外推尺寸")
	}
	p.Decoders[0].Mode = "invalid"
	if Apply(p) {
		t.Fatal("接受未知解碼偏好")
	}
	p.Decoders[0].Mode = "hardware"
	p.Decoders = append(p.Decoders, p.Decoders[0])
	if Apply(p) {
		t.Fatal("接受重複解碼策略")
	}
}
