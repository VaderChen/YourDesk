package hardwareprobe

import (
	"encoding/json"
	"testing"
)

func TestDetectorPolicyUnknownAndFailure(t *testing.T) {
	if policyFromState(State{Status: "running"}) != nil {
		t.Fatal("未完成策略不可套用")
	}
	s := State{Status: "partial", Results: []Result{
		{Key: "inventory", State: "complete", Data: json.RawMessage(`{"cpuFeatures":{"ASIMD":true}}`)},
		{Key: "h264", State: "complete", Data: json.RawMessage(`{"codec":"h264","input":"BGRA","width":128,"height":128,"create_status":0,"encode_status":0,"encode_callback_status":0,"sample_produced":1,"hardware_encoder":{"value":true}}`)},
		{Key: "hevc", State: "complete", Data: json.RawMessage(`{"codec":"hevc","input":"BGRA","width":128,"height":128,"create_status":0,"encode_status":0,"encode_callback_status":0,"sample_produced":1,"hardware_encoder":{"value":null}}`)},
	}}
	p := policyFromState(s)
	if p.CompareBytes != 64<<20 {
		t.Fatal("未套用 SIMD 快照預算")
	}
	if yes, known := p.EncoderDecision("h264", 128, 128); !known || !yes {
		t.Fatal("已確認能力遺失")
	}
	if _, known := p.EncoderDecision("hevc", 128, 128); known {
		t.Fatal("未知被當成確定結果")
	}
	s.Results[2].Data = json.RawMessage(`{"codec":"hevc","input":"BGRA","width":128,"height":128,"create_status":-1}`)
	if yes, known := policyFromState(s).EncoderDecision("hevc", 128, 128); !known || yes {
		t.Fatal("未排除實測失敗")
	}
	s.Results[2].State = "timeout"
	if _, known := policyFromState(s).EncoderDecision("hevc", 128, 128); known {
		t.Fatal("逾時被當成不支援")
	}
}
