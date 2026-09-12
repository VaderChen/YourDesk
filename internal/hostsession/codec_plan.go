package hostsession

import (
	"yourdesk/internal/optimization"
	"yourdesk/internal/video"
)

type codecAttempt struct {
	codec         video.WireCodec
	width, height int
}

// codecPlan 只納入正式 Host 可建立的編碼器；原生診斷矩陣不等於串流後端。
func codecPlan(policy optimization.Policy, requested video.Codec, width, height int, remote, remoteHardware uint32, failed map[codecAttempt]bool, supports func(video.Codec) bool, goal optimization.Goal) []video.WireCodec {
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
		if remote&(1<<wire) == 0 || failed[codecAttempt{wire, w, h}] {
			continue
		}
		usable, known := policy.EncoderDecision(entry.name, w, h)
		if known && !usable || !known && !supports(entry.codec) {
			continue
		}
		candidates = append(candidates, optimization.Configuration{Codec: entry.name, HardwareEncode: true, HardwareDecode: remoteHardware&remote&(1<<wire) != 0, ExactSizeVerified: known && usable})
	}
	result := []video.WireCodec{}
	for _, candidate := range optimization.RankConfigurations(candidates, goal) {
		result = append(result, codecs[candidate.Codec])
	}
	// JPEG 是現有相容回退路徑，不把尚未接入 Host 的軟體影片編碼器列入候選。
	return append(result, video.WireJPEG)
}
