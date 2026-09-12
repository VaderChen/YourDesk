//go:build windows && cgo

package hardwareprobe

import (
	"encoding/json"
	"strings"
	"yourdesk/internal/video"
	"yourdesk/internal/winmedia"
)

func nativeJobs() []job { return windowsNativeJobs() }
func nativeProbe(j job) ([]byte, error) {
	if j.Format == "software" {
		return json.Marshal(video.ProbeSoftwareCodec(j.Codec, j.Width, j.Height, j.Phase))
	}
	if j.Phase == "decode" {
		fixture, err := video.ProbeElementaryFixture(j.Codec, j.Width, j.Height)
		if err != nil {
			return nil, err
		}
		return json.Marshal(winmedia.ProbeDecode(j.Codec, j.Format, j.Width, j.Height, fixture))
	}
	return json.Marshal(winmedia.Probe(j.Codec, j.Format, j.Width, j.Height))
}

func nativeInventory(inventory map[string]any) {
	if gpus := winmedia.ProbeGraphics(); len(gpus) > 0 {
		api := preferGraphicsAPI(gpus)
		inventory["preferredGraphicsAPI"] = api
		if device := preferredGraphicsDevice(gpus, api); device != nil {
			inventory["preferredGraphicsFeatureLevel"] = device[strings.ToLower(api)+"FeatureLevel"]
			inventory["preferredGraphicsGPU"] = device["name"]
		}
		inventory["gpus"] = gpus
		for _, gpu := range gpus {
			if gpu["d3d12"] == nil || (gpu["d3d12"] == false && gpu["d3d11"] == nil) {
				inventory["graphicsDetectionIncomplete"] = true
			}
		}
	}
}

func runtimeDecodeProbe(j job) ([]byte, error) {
	r := video.ProbeReceiverCodec(j.Codec, j.Width, j.Height)
	r["codec"] = j.Codec
	r["input"] = j.Format
	r["width"] = j.Width
	r["height"] = j.Height
	r["phase"] = "decode"
	return json.Marshal(r)
}
