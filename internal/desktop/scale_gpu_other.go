//go:build (!darwin && !windows) || !cgo

package desktop

import "fmt"

func newGPUScaler() (gpuScaler, error) {
	return nil, fmt.Errorf("此平台尚未提供 GPU 縮圖後端")
}
