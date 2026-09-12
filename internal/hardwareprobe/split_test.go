package hardwareprobe

import (
	"encoding/json"
	"testing"
)

func TestIndependentDirections(t *testing.T) {
	jobs := splitJobs(append([]job{{}}, windowsNativeJobs()...))
	seen := map[string]bool{}
	for _, j := range jobs {
		if seen[key(j)] {
			t.Fatal("工作重複", key(j))
		}
		seen[key(j)] = true
	}
	for _, j := range windowsNativeJobs() {
		for _, phase := range []string{"encode", "decode"} {
			j.Phase = phase
			if !seen[key(j)] {
				t.Fatal("缺少獨立工作", key(j))
			}
		}
	}
	if len(jobs) != 65 {
		t.Fatal("未拆分所有原生／軟體方向", len(jobs))
	}
}
func TestDecodePolicySurvivesEncoderTimeout(t *testing.T) {
	s := State{Status: "partial", Results: []Result{
		{Key: "hevc/RGBA/1920x1080/encode", State: "timeout"},
		{Key: "hevc/RGBA/1920x1080/decode", State: "complete", Data: json.RawMessage(`{"codec":"hevc","input":"RGBA","width":1920,"height":1080,"phase":"decode","decodeOK":true,"decodingMode":"software"}`)},
	}}
	p := policyFromState(s)
	if len(p.Encoders) != 0 {
		t.Fatal("解碼證據被當成編碼證據")
	}
	if len(p.Decoders) != 1 || p.Decoders[0].Mode != "software" {
		t.Fatalf("編碼逾時影響獨立軟解證據：%+v", p)
	}
}
