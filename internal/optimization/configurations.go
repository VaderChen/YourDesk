package optimization

import "sort"

// Goal 只調整同一加速等級內的偏好，不覆蓋雙端／編碼端硬體優先規則。
type Goal string

const (
	Balanced   Goal = "balanced"
	LowLatency Goal = "low-latency"
	Bandwidth  Goal = "bandwidth"
)

// Configuration 是已確認雙端可用的路徑；未測尺寸仍需正式後端驗證。
// HardwareDecode 僅表示對端公告的能力，不宣稱已測目前串流尺寸。
type Configuration struct {
	Codec             string `json:"codec"`
	HardwareEncode    bool   `json:"hardwareEncode"`
	HardwareDecode    bool   `json:"hardwareDecode"`
	ExactSizeVerified bool   `json:"exactSizeVerified"`
}

// RankConfigurations 以離散優先序排序，避免任意加權抵消硬體優先規則。
// 同級先採用實測尺寸，再依用途偏好；不以 helper 耗時推論吞吐量。
func RankConfigurations(candidates []Configuration, goal Goal) []Configuration {
	order := []string{"hevc", "h264", "av1", "jpeg"}
	switch goal {
	case LowLatency:
		order = []string{"h264", "hevc", "av1", "jpeg"}
	case Bandwidth:
		order = []string{"av1", "hevc", "h264", "jpeg"}
	}
	priority := map[string]int{}
	for i, name := range order {
		priority[name] = i
	}
	result := make([]Configuration, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := priority[candidate.Codec]; ok {
			result = append(result, candidate)
		}
	}
	tier := func(c Configuration) int {
		if c.HardwareEncode {
			if c.HardwareDecode {
				return 0
			}
			return 1
		}
		if c.HardwareDecode {
			return 2
		}
		return 3
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if tier(a) != tier(b) {
			return tier(a) < tier(b)
		}
		if a.ExactSizeVerified != b.ExactSizeVerified {
			return a.ExactSizeVerified
		}
		return priority[a.Codec] < priority[b.Codec]
	})
	return result
}
