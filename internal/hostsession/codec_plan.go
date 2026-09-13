package hostsession

import (
	"yourdesk/internal/optimization"
	"yourdesk/internal/video"
)

type codecAttempt struct {
	codec         video.Codec
	width, height int
}

// codecPlan 只納入正式 Host 可建立的編碼器；原生診斷矩陣不等於串流後端。
func codecPlan(policy optimization.Policy, requested video.Codec, width, height int, remote, remoteHardware uint32, failed map[codecAttempt]bool, supports func(video.Codec) bool, goal optimization.Goal) []video.Codec {
	if requested == "" {
		requested = video.CodecAuto
	}
	w, h := (width+1)&^1, (height+1)&^1
	candidates := []optimization.Configuration{}
	codecs := map[string]video.WireCodec{"hevc": video.WireHEVC, "h264": video.WireH264, "av1": video.WireAV1}
	for _, entry := range []struct {
		name  string
		codec video.Codec
	}{{"hevc", video.CodecHardwareHEVC}, {"h264", video.CodecHardwareH264}, {"av1", video.CodecHardwareAV1}} {
		wire := codecs[entry.name]
		if requested != video.CodecAuto && requested != entry.codec {
			continue
		}
		if remote&(1<<wire) == 0 || failed[codecAttempt{entry.codec, w, h}] {
			continue
		}
		usable, known := policy.EncoderDecision(entry.name, w, h)
		if known && !usable || !known && !supports(entry.codec) {
			continue
		}
		candidates = append(candidates, optimization.Configuration{Codec: entry.name, HardwareEncode: true, HardwareDecode: remoteHardware&remote&(1<<wire) != 0, ExactSizeVerified: known && usable})
	}
	// AV1 CPU 編碼是獨立候選，不將軟體成功當成硬體能力。
	swUsable, swKnown := policy.EncoderDecision("software-av1", w, h)
	if (requested == video.CodecAuto || requested == video.CodecSoftwareAV1) && remote&(1<<video.WireAV1) != 0 && !failed[codecAttempt{video.CodecSoftwareAV1, w, h}] && ((swKnown && swUsable) || (!swKnown && supports(video.CodecSoftwareAV1))) {
		candidates = append(candidates, optimization.Configuration{Codec: "av1", ExactSizeVerified: swKnown && swUsable, HardwareDecode: remoteHardware&remote&(1<<video.WireAV1) != 0})
	}
	result := []video.Codec{}
	for _, candidate := range optimization.RankConfigurations(candidates, goal) {
		chosen := map[string]video.Codec{"h264": video.CodecHardwareH264, "hevc": video.CodecHardwareHEVC, "av1": video.CodecHardwareAV1}[candidate.Codec]
		if candidate.Codec == "av1" && !candidate.HardwareEncode {
			chosen = video.CodecSoftwareAV1
		}
		result = append(result, chosen)
	}
	// JPEG 保留為舊接收端與所有影片候選失敗時的相容回退。
	return append(result, video.CodecSoftwareJPEG)
}
