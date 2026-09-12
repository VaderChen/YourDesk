//go:build windows && cgo && ffmpeg

package softwarevideo

// RuntimeFiles 必須隨 EXE 複製到受保護的登入前服務目錄。
func RuntimeFiles() []string { return []string{"avcodec-62.dll", "avutil-60.dll", "swscale-9.dll"} }
