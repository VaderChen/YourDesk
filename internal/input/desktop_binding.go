package input

import (
	"fmt"
	"strings"
)

type inputDesktop struct {
	handle uintptr
	name   string
}

// 僅由固定的輸入執行緒存取。舊桌面保留到新桌面綁定成功，
// 不在授權失敗後繼續把輸入送往舊桌面，也不保存事件等待日後重播。
type desktopBinding struct {
	open    func() (inputDesktop, error)
	bind    func(uintptr) error
	close   func(uintptr)
	current inputDesktop
}

func (b *desktopBinding) run(inject func() error) error {
	next, err := b.open()
	if err != nil {
		return fmt.Errorf("無法取得目前輸入桌面：%w", err)
	}
	// 同一 window station 內桌面名稱唯一；同桌面只關閉新開啟的
	// 查詢 handle，避免不斷重新綁定仍在使用的桌面 handle。
	if b.current.handle != 0 && strings.EqualFold(next.name, b.current.name) {
		b.close(next.handle)
		return inject()
	}
	if err := b.bind(next.handle); err != nil {
		b.close(next.handle)
		return fmt.Errorf("無法切換輸入執行緒的桌面：%w", err)
	}
	previous := b.current
	b.current = next
	if previous.handle != 0 {
		b.close(previous.handle)
	}
	return inject()
}
