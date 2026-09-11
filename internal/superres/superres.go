// Package superres 管理 遠端顯示 超解析度後端；探測與推論都在背景執行。
package superres

import (
	"fmt"
	"image"
	"sync"
)

type Capability struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}

var once sync.Once
var mu sync.RWMutex
var inferenceMu sync.Mutex
var models = []Capability{
	{ID: "quicksrnet-small", Name: "QuickSRNet Small 2×", Reason: "正在偵測"},
	{ID: "sesr-m5", Name: "SESR M5 2×", Reason: "正在偵測"},
}

func StartProbe() {
	once.Do(func() {
		go func() {
			for i := range models {
				err := loadModel(i)
				mu.Lock()
				models[i].Available = err == nil
				models[i].Reason = ""
				if err != nil {
					models[i].Reason = err.Error()
				}
				mu.Unlock()
			}
		}()
	})
}
func Models() []Capability {
	StartProbe()
	mu.RLock()
	defer mu.RUnlock()
	return append([]Capability(nil), models...)
}
func Capabilities() []Capability {
	entries := Models()
	core := Capability{ID: "coreml", Name: "Core ML", Reason: entries[0].Reason}
	for _, m := range entries {
		if m.Available {
			core.Available = true
			core.Reason = ""
		}
	}
	return []Capability{{ID: "fsr1", Name: "FSR 1 · EASU + RCAS", Available: true}, core}
}
func ModelName(id int) string {
	mu.RLock()
	defer mu.RUnlock()
	if id < 0 || id >= len(models) {
		return "Core ML"
	}
	return models[id].Name
}
func Ready(id int) bool {
	mu.RLock()
	defer mu.RUnlock()
	return id >= 0 && id < len(models) && models[id].Available
}
func Enhance(id int, src *image.RGBA) (*image.RGBA, error) {
	if !Ready(id) {
		return nil, fmt.Errorf("Core ML 模型尚未就緒")
	}
	inferenceMu.Lock()
	defer inferenceMu.Unlock()
	return predict(id, src)
}
