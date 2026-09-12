package hardwareprobe

import "testing"

func TestPreferGraphicsAPI(t *testing.T) {
	for _, tc := range []struct {
		name    string
		devices []map[string]any
		want    string
	}{
		{"兩者支援優先12", []map[string]any{{"d3d11": true, "d3d12": true}}, "D3D12"},
		{"只有11", []map[string]any{{"d3d11": true, "d3d12": false}}, "D3D11"},
		{"只有12", []map[string]any{{"d3d11": false, "d3d12": true}}, "D3D12"},
		{"都不支援", []map[string]any{{"d3d11": false, "d3d12": false}}, ""},
		{"缺少證據", []map[string]any{{}}, ""},
		{"多卡不降級", []map[string]any{{"d3d12": true}, {"d3d11": true, "d3d12": false}}, "D3D12"},
		{"12未知不可降級", []map[string]any{{"d3d11": true, "d3d12": nil}}, ""},
		{"另一張卡12未知不降級", []map[string]any{{"d3d11": true, "d3d12": false}, {"d3d12": nil}}, ""},
		{"多卡升級", []map[string]any{{"d3d11": true, "d3d12": false}, {"d3d12": true}}, "D3D12"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := preferGraphicsAPI(tc.devices); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
			for _, d := range tc.devices {
				if _, ok := d["preferredGraphicsAPI"]; !ok {
					t.Fatal("逐卡結果遺失")
				}
			}
		})
	}
}

func TestPreferredGraphicsFeatureLevel(t *testing.T) {
	devices := []map[string]any{
		{"name": "舊卡", "d3d12": true, "d3d12FeatureLevel": "12.0"},
		{"name": "高版號", "d3d12": true, "d3d12FeatureLevel": "12.2"},
		{"name": "中版號", "d3d12": true, "d3d12FeatureLevel": "12.1"},
		{"name": "不支援", "d3d12": false, "d3d12FeatureLevel": "13.0"},
		{"name": "缺值", "d3d12": true},
	}
	if got := preferredGraphicsDevice(devices, "D3D12"); got["name"] != "高版號" {
		t.Fatalf("未選最高可用 FL：%v", got)
	}
	devices[2]["d3d12FeatureLevel"] = "12.10"
	if got := preferredGraphicsDevice(devices, "D3D12"); got["name"] != "中版號" {
		t.Fatal("FL 應依數值而非字串比較")
	}
	devices[2]["d3d12FeatureLevel"] = "12.2"
	if got := preferredGraphicsDevice(devices, "D3D12"); got["name"] != "高版號" {
		t.Fatal("同版號不應任意切換")
	}
	if preferredGraphicsDevice(devices, "") != nil {
		t.Fatal("未確認 API 不可選 FL")
	}
	fallback := []map[string]any{
		{"name": "未知12", "d3d11": true, "d3d11FeatureLevel": "12.1"},
		{"name": "確認可退回", "d3d12": false, "d3d11": true, "d3d11FeatureLevel": "11.1"},
	}
	if got := preferredGraphicsDevice(fallback, "D3D11"); got["name"] != "確認可退回" {
		t.Fatal("未確認12不應降級")
	}
}
