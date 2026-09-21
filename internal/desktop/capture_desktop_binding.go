package desktop

import "strings"

// 只由固定的擷取執行緒使用；切換前釋放舊桌面資源，失敗時不擷取舊畫面。
type captureDesktopBinding struct {
	open    func() (uintptr, string, error)
	bind    func(uintptr) error
	release func(uintptr)
	current uintptr
	name    string
}

func (b *captureDesktopBinding) ensure(reset func()) error {
	h, name, err := b.open()
	if err != nil {
		return err
	}
	if b.current != 0 && strings.EqualFold(name, b.name) {
		b.release(h)
		return nil
	}
	reset()
	if err := b.bind(h); err != nil {
		b.release(h)
		return err
	}
	previous := b.current
	b.current, b.name = h, name
	if previous != 0 {
		b.release(previous)
	}
	return nil
}
