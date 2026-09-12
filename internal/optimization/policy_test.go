package optimization

import (
	"runtime"
	"testing"
)

func TestPolicyValidationAndSnapshot(t *testing.T) {
	old := current.Load()
	defer current.Store(old)
	p := Policy{Version: 1, OS: runtime.GOOS, Arch: runtime.GOARCH, CompareBytes: 64 << 20, Encoders: []Encoder{{"h264", 128, 128, true}}}
	if !Apply(p) {
		t.Fatal("策略被拒絕")
	}
	p.Encoders[0].Usable = false
	s := Snapshot()
	if usable, known := s.EncoderDecision("h264", 128, 128); !usable || !known {
		t.Fatal("快照被呼叫端修改")
	}
	if _, known := s.EncoderDecision("h264", 1920, 1080); known {
		t.Fatal("錯誤外推尺寸")
	}
	p.CompareBytes = 65 << 20
	if Apply(p) {
		t.Fatal("接受過大記憶體預算")
	}
	p.CompareBytes = 0
	p.OS = "other"
	if Apply(p) {
		t.Fatal("接受跨平台策略")
	}
}
