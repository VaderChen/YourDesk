// Package frameinterp 提供 RIFE 補幀後端，與 遠端顯示 排程分離。
package frameinterp

import (
	"context"
	"fmt"
	"image"
	"sync"
)

var once sync.Once
var lock sync.RWMutex
var inference sync.Mutex
var ready bool
var reason = "RIFE 正在載入"

func Start() {
	once.Do(func() {
		go func() {
			err := load()
			lock.Lock()
			defer lock.Unlock()
			ready = err == nil
			reason = ""
			if err != nil {
				reason = err.Error()
			}
		}()
	})
}
func Status() (bool, string) { Start(); lock.RLock(); defer lock.RUnlock(); return ready, reason }
func Midpoint(ctx context.Context, a, b *image.RGBA) (*image.RGBA, error) {
	if ok, msg := Status(); !ok {
		return nil, fmt.Errorf("%s", msg)
	}
	inference.Lock()
	defer inference.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a.Bounds() != b.Bounds() || a.Bounds().Empty() {
		return nil, fmt.Errorf("RIFE 影格尺寸不一致")
	}
	result, err := predict(a, b)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return result, err
}
