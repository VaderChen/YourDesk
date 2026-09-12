package hardwareprobe

import (
	"encoding/json"
	"testing"
)

func TestWindowsNativeMatrix(t *testing.T) {
	jobs := windowsNativeJobs()
	if len(jobs) != 32 {
		t.Fatalf("原生矩陣缺項：%d", len(jobs))
	}
	for _, codec := range []string{"jpeg", "h264", "hevc", "av1"} {
		count := 0
		for _, j := range jobs {
			if j.Codec == codec && j.Format == "software" {
				count++
			}
		}
		if count != 2 {
			t.Fatalf("%s 缺少獨立軟體測試", codec)
		}
	}
	seen := map[string]bool{}
	for _, j := range jobs {
		if seen[key(j)] {
			t.Fatalf("重複工作：%s", key(j))
		}
		seen[key(j)] = true
	}
	for _, codec := range []string{"jpeg", "h264", "hevc", "av1"} {
		for _, format := range []string{"BGRA", "NV12-full", "NV12-video"} {
			for _, size := range [][2]int{{128, 128}, {1920, 1080}} {
				if !seen[key(job{"", codec, format, size[0], size[1]})] {
					t.Fatal("格式與尺寸矩陣缺項")
				}
			}
		}
	}
}
func TestNativeEvidenceDoesNotOverrideRuntimePolicy(t *testing.T) {
	s := State{Status: "complete", Results: []Result{
		{Key: "hevc/BGRA/128x128", State: "complete", Data: json.RawMessage(`{"probeKind":"windows-native","codec":"hevc","input":"BGRA","width":128,"height":128,"encode_status":-1,"decode_status":-1}`)},
		{Key: "h264/RGBA/128x128", State: "complete", Data: json.RawMessage(`{"codec":"h264","input":"RGBA","width":128,"height":128,"encodeOK":true,"hardwareEncoder":true}`)},
	}}
	p := policyFromState(s)
	if len(p.Encoders) != 1 || len(p.Decoders) != 0 {
		t.Fatalf("原生證據污染正式串流策略：%+v", p)
	}
	if usable, known := p.EncoderDecision("h264", 128, 128); !known || !usable {
		t.Fatal("正式路徑證據遺失")
	}
}

func TestIndependentSoftwareEvidencePreservesHardware(t *testing.T) {
	for _, mode := range []string{"hardware", "unavailable"} {
		s := State{Status: "complete", Results: []Result{
			{State: "complete", Data: json.RawMessage(`{"codec":"h264","input":"RGBA","width":128,"height":128,"encodeOK":true,"decodeOK":` + map[string]string{"hardware": "true", "unavailable": "false"}[mode] + `,"decodingMode":"` + mode + `"}`)},
			{State: "complete", Data: json.RawMessage(`{"probeKind":"windows-software","codec":"h264","input":"software","width":128,"height":128,"decodeOK":true}`)},
		}}
		p := policyFromState(s)
		want := mode
		if want == "unavailable" {
			want = "software"
		}
		if len(p.Decoders) != 1 || p.DecoderDecision("h264", 128, 128) != want {
			t.Fatalf("獨立軟解證據合併錯誤：%+v", p)
		}
	}
}
