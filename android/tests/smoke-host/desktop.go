package main

import (
	"context"
	"fmt"
	"image/color"
	"sync"
	"time"
	"yourdesk/internal/p2p"
)

// 固定合成螢幕與輸入紀錄，用正式 DataChannel 驗證手機操作，不注入桌面 OS。
type desktopSmoke struct {
	mu                       sync.Mutex
	count, current, previous int
	request                  uint64
	next                     *p2p.Control
	due, lagUntil            time.Time
	controls                 []p2p.Control
	barriers, acknowledged   uint64
	frames                   [][]byte
}

func newDesktopSmoke() *desktopSmoke {
	return &desktopSmoke{count: 3, frames: [][]byte{nil, frame(color.RGBA{230, 200, 20, 255}), frame(color.RGBA{180, 20, 210, 255})}}
}
func (d *desktopSmoke) control(c p2p.Control) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c.Type == "smoke-barrier" {
		d.barriers++
		return
	}
	if c.Type == "display-select" {
		copy := c
		d.next = &copy
		d.due = time.Now().Add(400 * time.Millisecond)
		return
	}
	if c.Type == "button" || c.Type == "move" || c.Type == "wheel" {
		if len(d.controls) < 256 {
			d.controls = append(d.controls, c)
		}
	}
}
func (d *desktopSmoke) attach(peer *p2p.Peer) {
	_ = peer.RegisterCommand("smoke.barrier", func(ctx context.Context) (any, error) {
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		for {
			d.mu.Lock()
			ready := d.barriers > d.acknowledged
			if ready {
				d.acknowledged = d.barriers
			}
			d.mu.Unlock()
			if ready {
				return nil, nil
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-tick.C:
			}
		}
	})
	register := func(name string, fn func() error) {
		if err := peer.RegisterCommand(name, func(context.Context) (any, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			return nil, fn()
		}); err != nil {
			panic(err)
		}
	}
	register("smoke.input.reset", func() error { d.controls = nil; return nil })
	register("smoke.input.clean", func() error {
		if len(d.controls) != 0 {
			return fmt.Errorf("非預期輸入：%+v", d.controls)
		}
		return nil
	})
	for _, button := range []int{1, 2, 3} {
		register(fmt.Sprintf("smoke.input.button%d", button), func() error {
			var buttons []p2p.Control
			for _, c := range d.controls {
				if c.Display == nil || *c.Display != d.current {
					return fmt.Errorf("輸入未綁定目前螢幕")
				}
				if c.Type == "wheel" {
					return fmt.Errorf("點擊混入滾輪")
				}
				if c.Type == "button" {
					buttons = append(buttons, c)
				}
			}
			if len(buttons) != 2 || buttons[0].Button != button || !buttons[0].Down || buttons[1].Button != button || buttons[1].Down {
				return fmt.Errorf("按鍵配對錯誤：%+v", buttons)
			}
			return nil
		})
	}
	for _, sign := range []int{-1, 1} {
		name := "down"
		if sign > 0 {
			name = "up"
		}
		register("smoke.input.wheel-"+name, func() error {
			wheels := 0
			for i, c := range d.controls {
				if c.Type == "button" {
					return fmt.Errorf("捲動誤觸按鍵")
				}
				if c.Type != "wheel" {
					continue
				}
				wheels++
				if c.Delta*float64(sign) <= 0 || c.Display == nil || *c.Display != d.current {
					return fmt.Errorf("捲動方向或螢幕錯誤")
				}
				if i == 0 || d.controls[i-1].Type != "move" || d.controls[i-1].X != c.X || d.controls[i-1].Y != c.Y {
					return fmt.Errorf("捲動前未定位游標")
				}
			}
			if wheels == 0 {
				return fmt.Errorf("未收到捲動")
			}
			return nil
		})
	}
	register("smoke.displays.remove", func() error { d.count = 1; d.current = 0; d.next = nil; return nil })
	register("smoke.displays.none", func() error { d.count = 0; d.current = 0; return nil })
}
func (d *desktopSmoke) send(peer *p2p.Peer, sequence uint64, primary []byte) {
	d.mu.Lock()
	if d.next != nil && time.Now().After(d.due) {
		d.previous = d.current
		if d.next.Display != nil && *d.next.Display >= 0 && *d.next.Display < d.count {
			d.current = *d.next.Display
		}
		d.request = d.next.DisplayRequest
		d.next = nil
		d.lagUntil = time.Now().Add(600 * time.Millisecond)
	}
	current, count, request := d.current, d.count, d.request
	shown := current
	if time.Now().Before(d.lagUntil) {
		shown = d.previous
	}
	payload := primary
	if shown > 0 {
		payload = d.frames[shown]
	}
	d.mu.Unlock()
	_ = peer.SendControl(p2p.Control{Type: "displays", Display: &current, DisplayCount: count, DisplayRequest: request})
	if count > 0 {
		_ = peer.SendFrame(p2p.Frame{Sequence: sequence, Display: shown, Width: 640, Height: 360, Keyframe: true, JPEG: payload})
	}
}
