//go:build !windows || !cgo || !ffmpeg

package softwarevideo

func RuntimeFiles() []string { return nil }
