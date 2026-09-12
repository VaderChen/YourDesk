//go:build darwin && cgo

package hardwareprobe

import (
	"encoding/json"
	"testing"
)

func TestDarwinIndependentDecodeSmoke(t *testing.T) {
	jobs := platformJobs()
	if len(jobs) != 37 {
		t.Fatalf("工作數錯誤：%d", len(jobs))
	}
	for _, j := range jobs {
		if j.Phase != "decode" {
			continue
		}
		t.Run(key(j), func(t *testing.T) {
			data, err := platformProbe(j)
			if err != nil {
				t.Fatal(err)
			}
			var r map[string]any
			if err = json.Unmarshal(data, &r); err != nil {
				t.Fatal(err)
			}
			if r["fixture_status"] != float64(0) || r["decoderSource"] != "independent-fixture" || r["phase"] != "decode" {
				t.Fatalf("獨立樣本建立失敗：%s", data)
			}
			for _, field := range []string{"create_status", "encode_status", "sample_produced", "hardware_encoder"} {
				if _, ok := r[field]; ok {
					t.Fatalf("解碼混入編碼欄位：%s", field)
				}
			}
			t.Logf("解碼成功=%v，建立狀態=%v，輸出=%v", r["decodeOK"], r["decoder_create_status"], r["decoded_format"])
		})
	}
}

func TestDarwinIndependentEncodeSmoke(t *testing.T) {
	for _, j := range platformJobs() {
		if j.Phase != "encode" {
			continue
		}
		t.Run(key(j), func(t *testing.T) {
			data, err := platformProbe(j)
			if err != nil {
				t.Fatal(err)
			}
			var r map[string]any
			if err = json.Unmarshal(data, &r); err != nil {
				t.Fatal(err)
			}
			if r["phase"] != "encode" {
				t.Fatalf("編碼方向錯誤：%s", data)
			}
			for _, field := range []string{"decoder_create_status", "decode_status", "decodeOK", "decoded_size_matches", "hardware_decoder", "decoderSource"} {
				if _, ok := r[field]; ok {
					t.Fatalf("編碼混入解碼欄位：%s", field)
				}
			}
		})
	}
}
