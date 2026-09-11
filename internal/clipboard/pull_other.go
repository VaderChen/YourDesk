//go:build (!darwin && !windows) || !cgo

package clipboard

import "errors"

func nativePullSupported() bool           { return false }
func nativePublishOffer(*pullOffer) error { return errors.New("此平台不支援延遲檔案貼上") }
