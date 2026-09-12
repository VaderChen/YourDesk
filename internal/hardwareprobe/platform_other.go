//go:build !darwin || !cgo

package hardwareprobe

import (
	"encoding/json"
	"golang.org/x/sys/cpu"
	"image"
	"runtime"
	"time"
	"yourdesk/internal/video"
)

// 非 macOS 平台驗證正式 Go 後端目前接受的 RGBA；不假稱已測 BGRA/NV12 原生輸入。
func platformJobs() []job {
	jobs := []job{{}}
	codecs := []string{"jpeg", "h264", "hevc"}
	if runtime.GOOS == "windows" {
		codecs = append(codecs, "av1")
	}
	for _, codec := range codecs {
		for _, size := range [][2]int{{128, 128}, {1920, 1080}} {
			jobs = append(jobs, job{"", codec, "RGBA", size[0], size[1]})
		}
	}
	jobs = append(jobs, nativeJobs()...)
	if runtime.GOOS == "windows" {
		return splitJobs(jobs)
	}
	return jobs
}
func platformProbe(j job) ([]byte, error) {
	if j.Codec != "" && j.Format != "RGBA" {
		return nativeProbe(j)
	}
	if j.Phase == "decode" {
		return runtimeDecodeProbe(j)
	}
	start := time.Now()
	r := map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH}
	if j.Codec == "" {
		features := map[string]bool{}
		switch runtime.GOARCH {
		case "amd64", "386":
			features = map[string]bool{"SSE2": cpu.X86.HasSSE2, "SSSE3": cpu.X86.HasSSSE3, "AVX2": cpu.X86.HasAVX2, "AVX512F": cpu.X86.HasAVX512F}
		case "arm64":
			features = map[string]bool{"ASIMD": cpu.ARM64.HasASIMD, "ASIMDDP": cpu.ARM64.HasASIMDDP}
		}
		r["logicalCPUs"] = runtime.NumCPU()
		r["memoryBytes"] = physicalMemoryBytes()
		r["cpuFeatures"] = features
		r["cpu"], r["gpus"] = platformDeviceNames()
		nativeInventory(r)
		r["power"] = nil
		r["detail"] = "裝置名稱依平台查詢結果提供；電源清單尚未實作，另以正式編解碼後端驗證 RGBA 工作負載"
	} else {
		r["codec"] = j.Codec
		r["input"] = j.Format
		r["width"] = j.Width
		r["height"] = j.Height
		pixels := image.NewRGBA(image.Rect(0, 0, j.Width, j.Height))
		for i := 3; i < len(pixels.Pix); i += 4 {
			pixels.Pix[i] = 255
		}
		if j.Codec == "jpeg" {
			enc, selection := video.NewJPEGEncoder(true)
			defer enc.Close()
			data, err := enc.Encode(pixels, 70)
			r["encodeOK"] = err == nil && len(data) > 0
			r["hardwareEncoder"] = enc.Hardware()
			r["backend"] = enc.Backend()
			r["selection"] = selection.Selected
			if err == nil && j.Phase == "" {
				dec, _ := video.NewJPEGDecoder(true)
				defer dec.Close()
				im, e := dec.Decode(data)
				r["decodeOK"] = e == nil && im != nil && im.Bounds().Dx() == j.Width && im.Bounds().Dy() == j.Height
				r["hardwareDecoder"] = dec.Hardware()
			}
		} else {
			codec := video.CodecHardwareH264
			if j.Codec == "av1" {
				codec = video.CodecHardwareAV1
			}
			if j.Codec == "hevc" {
				codec = video.CodecHardwareHEVC
			}
			enc, err := video.NewIntraEncoder(codec)
			r["hardwareEncoder"] = nil
			r["hardwareDecoder"] = nil
			if err != nil {
				r["status"] = "unavailable"
			} else {
				defer enc.Close()
				data, e := enc.Encode(pixels, 70)
				r["encodeOK"] = e == nil && len(data) > 0
				// Windows 正式影片編碼器只選擇硬體 MFT，成功產出才確認硬體可用。
				if runtime.GOOS == "windows" && e == nil && len(data) > 0 {
					r["hardwareEncoder"] = true
				}
				if b, ok := enc.(video.BackendReporter); ok {
					r["encoderBackend"] = b.Backend()
				}
				if e == nil && runtime.GOOS != "windows" {
					var dec video.DecodeSession
					defer dec.Close()
					im, de := dec.Decode(video.WireForCodec(codec), data)
					r["decodeOK"] = de == nil && im != nil && im.Bounds().Dx() == j.Width && im.Bounds().Dy() == j.Height
					r["decoderBackend"] = dec.Backend()
					r["decodingMode"] = dec.DecodingMode()
				}
			}
		}
	}
	r["phase"] = j.Phase
	r["durationMS"] = float64(time.Since(start).Microseconds()) / 1000
	return json.Marshal(r)
}
