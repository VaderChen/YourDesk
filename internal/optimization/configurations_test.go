package optimization

import "testing"

func TestConfigurationGoals(t *testing.T) {
	all := []Configuration{{Codec: "h264", HardwareEncode: true, HardwareDecode: true}, {Codec: "hevc", HardwareEncode: true, HardwareDecode: true}, {Codec: "av1", HardwareEncode: true, HardwareDecode: true}}
	for goal, want := range map[Goal]string{Balanced: "av1", LowLatency: "h264", Bandwidth: "av1"} {
		if got := RankConfigurations(all, goal)[0].Codec; got != want {
			t.Fatalf("%s: %s", goal, got)
		}
	}
	if all[0].Codec != "h264" {
		t.Fatal("修改了輸入")
	}
}
func TestConfigurationHardwarePriority(t *testing.T) {
	all := []Configuration{{Codec: "av1"}, {Codec: "av1", HardwareDecode: true}, {Codec: "av1", HardwareEncode: true}, {Codec: "h264", HardwareEncode: true, HardwareDecode: true}}
	for _, goal := range []Goal{Balanced, LowLatency, Bandwidth} {
		got := RankConfigurations(all, goal)
		if !got[0].HardwareEncode || !got[0].HardwareDecode || !got[1].HardwareEncode || !got[2].HardwareDecode {
			t.Fatalf("模式覆蓋硬體優先：%+v", got)
		}
	}
}
