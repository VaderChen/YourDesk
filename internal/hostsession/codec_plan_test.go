package hostsession

import (
	"testing"
	"yourdesk/internal/optimization"
	"yourdesk/internal/video"
)

func TestCodecPlanFallbackAndEvidence(t *testing.T) {
	all := uint32(1<<video.WireH264 | 1<<video.WireHEVC | 1<<video.WireAV1)
	supports := func(video.Codec) bool { return true }
	failed := map[codecAttempt]bool{{video.CodecHardwareHEVC, 1920, 1080}: true}
	p := optimization.Policy{Encoders: []optimization.Encoder{{Codec: "av1", Width: 1920, Height: 1080, Usable: false}}}
	got := codecPlan(p, video.CodecAuto, 1920, 1080, all, all, failed, supports, optimization.Balanced)
	if len(got) != 3 || got[0] != video.CodecHardwareH264 || got[1] != video.CodecSoftwareAV1 || got[2] != video.CodecSoftwareJPEG {
		t.Fatalf("單一失敗未逐項回退：%v", got)
	}
	got = codecPlan(p, video.CodecAuto, 128, 128, all, all, failed, supports, optimization.Balanced)
	if len(got) < 2 || got[0] != video.CodecHardwareAV1 || got[1] != video.CodecHardwareHEVC {
		t.Fatal("失敗證據錯誤外推到另一尺寸")
	}
	got = codecPlan(p, video.CodecHardwareAV1, 1920, 1080, all, all, failed, supports, optimization.Balanced)
	if len(got) != 1 || got[0] != video.CodecSoftwareJPEG {
		t.Fatal("覆寫使用者指定格式")
	}
	got = codecPlan(p, video.CodecAuto, 128, 128, 0, all, nil, supports, optimization.Balanced)
	if len(got) != 1 || got[0] != video.CodecSoftwareJPEG {
		t.Fatal("未公告解碼就送影片")
	}
}
func TestCodecPlanGoalAndExactSize(t *testing.T) {
	all := uint32(1<<video.WireH264 | 1<<video.WireHEVC | 1<<video.WireAV1)
	supports := func(video.Codec) bool { return true }
	for goal, want := range map[optimization.Goal]video.Codec{optimization.Balanced: video.CodecHardwareAV1, optimization.LowLatency: video.CodecHardwareH264, optimization.Bandwidth: video.CodecHardwareAV1} {
		got := codecPlan(optimization.Policy{}, video.CodecAuto, 1920, 1080, all, all, nil, supports, goal)
		if got[0] != want {
			t.Fatalf("%s: %v", goal, got)
		}
	}
	p := optimization.Policy{Encoders: []optimization.Encoder{{Codec: "h264", Width: 1920, Height: 1080, Usable: true}}}
	got := codecPlan(p, video.CodecAuto, 1920, 1080, all, all, nil, supports, optimization.Bandwidth)
	if got[0] != video.CodecHardwareH264 {
		t.Fatal("忽略已實測尺寸")
	}
}

func TestAV1HardwareFailureKeepsSoftwareCandidate(t *testing.T) {
	all := uint32(1 << video.WireAV1)
	failed := map[codecAttempt]bool{{video.CodecHardwareAV1, 1920, 1080}: true}
	supports := func(video.Codec) bool { return true }
	got := codecPlan(optimization.Policy{}, video.CodecAuto, 1920, 1080, all, all, failed, supports, optimization.Bandwidth)
	if len(got) != 2 || got[0] != video.CodecSoftwareAV1 {
		t.Fatalf("硬編失敗排除了軟編：%v", got)
	}
	p := optimization.Policy{Encoders: []optimization.Encoder{{Codec: "software-av1", Width: 1920, Height: 1080, Usable: false}}}
	got = codecPlan(p, video.CodecSoftwareAV1, 1920, 1080, all, all, nil, supports, optimization.Balanced)
	if len(got) != 1 || got[0] != video.CodecSoftwareJPEG {
		t.Fatalf("忽略此尺寸的軟編失敗：%v", got)
	}
	got = codecPlan(p, video.CodecSoftwareAV1, 128, 128, all, all, nil, supports, optimization.Balanced)
	if got[0] != video.CodecSoftwareAV1 {
		t.Fatalf("尺寸證據錯誤外推：%v", got)
	}
}
