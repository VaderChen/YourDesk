package hardwareprobe

import (
	"strconv"
	"strings"
)

// 依實際建立裝置的結果選擇較新的 API，並保留逐卡證據。
func preferGraphicsAPI(devices []map[string]any) string {
	preferred := ""
	d3d12Unknown := false
	for _, device := range devices {
		api := ""
		if device["d3d12"] == nil {
			d3d12Unknown = true
		}
		if device["d3d12"] == true {
			api = "D3D12"
		} else if device["d3d12"] == false && device["d3d11"] == true {
			api = "D3D11"
		}
		device["preferredGraphicsAPI"] = api
		if api == "D3D12" || preferred == "" {
			preferred = api
		}
	}
	if preferred != "D3D12" && d3d12Unknown {
		return ""
	}
	return preferred
}

// 在已選 API 的可用裝置中比較數值版號，缺少或無效 FL 不參與排序。
func preferredGraphicsDevice(devices []map[string]any, api string) map[string]any {
	if api != "D3D11" && api != "D3D12" {
		return nil
	}
	key := strings.ToLower(api)
	var chosen map[string]any
	bestMajor, bestMinor := -1, -1
	for _, device := range devices {
		if device[key] != true {
			continue
		}
		if api == "D3D11" && device["d3d12"] != false {
			continue
		}
		level, _ := device[key+"FeatureLevel"].(string)
		parts := strings.Split(level, ".")
		if len(parts) != 2 {
			continue
		}
		major, e1 := strconv.Atoi(parts[0])
		minor, e2 := strconv.Atoi(parts[1])
		if e1 != nil || e2 != nil || major <= 0 || minor < 0 {
			continue
		}
		if major > bestMajor || (major == bestMajor && minor > bestMinor) {
			chosen = device
			bestMajor = major
			bestMinor = minor
		}
	}
	return chosen
}
