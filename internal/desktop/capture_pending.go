package desktop

import "image"

// 原生擷取先確認並保留新影格，成功後才配置 Go 像素；無更新時不配置整張畫面。
// take 成功的資源一定在 copy 結束／失敗後 release，不會讓下一幀覆寫輸出。
func capturePendingRGBA(bounds image.Rectangle, take func() error, copyPixels func(*image.RGBA) error, release func()) (*image.RGBA, error) {
	if err := take(); err != nil {
		return nil, err
	}
	defer release()
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	if err := copyPixels(out); err != nil {
		return nil, err
	}
	return out, nil
}
