//go:build !windows || !cgo

package hardwareprobe

import "errors"

func nativeJobs() []job { return nil }
func nativeProbe(job) ([]byte, error) {
	return nil, errors.New("此建置未提供 Windows 原生探測")
}

func nativeInventory(map[string]any) {}

func runtimeDecodeProbe(job) ([]byte, error) {
	return nil, errors.New("此建置未提供獨立接收端探測")
}
