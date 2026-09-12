package hardwareprobe

// Windows 原生矩陣與正式 RGBA 路徑分開，使用同一份清單建立及驗證 helper 請求。
func windowsNativeJobs() []job {
	var jobs []job
	for _, codec := range []string{"jpeg", "h264", "hevc", "av1"} {
		for _, format := range []string{"BGRA", "NV12-full", "NV12-video"} {
			for _, size := range [][2]int{{128, 128}, {1920, 1080}} {
				jobs = append(jobs, job{"", codec, format, size[0], size[1]})
			}
		}
	}
	for _, codec := range []string{"jpeg", "h264", "hevc", "av1"} {
		for _, size := range [][2]int{{128, 128}, {1920, 1080}} {
			jobs = append(jobs, job{"", codec, "software", size[0], size[1]})
		}
	}
	return jobs
}
