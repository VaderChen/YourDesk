package hardwareprobe

import (
	"encoding/json"
	"testing"
)

func TestDetectorDecoderEvidence(t *testing.T) {
	for _, tc := range []struct{ name, evidence, mode string }{
		{"硬解確認", `"input":"BGRA","decoder_create_status":0,"decode_status":0,"decode_callback_status":0,"decoded_size_matches":1,"hardware_decoder":{"value":true}`, "hardware"},
		{"硬解建立失敗僅偏好軟解", `"input":"BGRA","decoder_create_status":-1`, "software"},
		{"缺少硬解證據", `"input":"BGRA","decoder_create_status":0,"decode_status":0,"decode_callback_status":0,"decoded_size_matches":1,"hardware_decoder":{"value":null}`, ""},
		{"編碼失敗不否定解碼", `"input":"BGRA","create_status":-1`, ""},
		{"完整路徑失敗", `"input":"RGBA","decodeOK":false`, "unavailable"},
		{"完整路徑成功", `"input":"RGBA","decodeOK":true,"decodingMode":"hardware"`, "hardware"},
		{"未知模式但解碼成功", `"input":"RGBA","decodeOK":true`, "auto"},
		{"缺少解碼結果", `"input":"RGBA","status":"unavailable"`, ""},
		{"null 不是失敗", `"input":"RGBA","decodeOK":null`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := State{Status: "complete", Results: []Result{{State: "complete", Data: json.RawMessage(`{"codec":"h264","width":128,"height":128,` + tc.evidence + `}`)}}}
			p := policyFromState(s)
			if got := p.DecoderDecision("h264", 128, 128); got != tc.mode {
				t.Fatalf("取得 %q，預期 %q", got, tc.mode)
			}
			s.Results[0].State = "timeout"
			if policyFromState(s).DecoderDecision("h264", 128, 128) != "" {
				t.Fatal("逾時被當成能力證據")
			}
		})
	}
}
