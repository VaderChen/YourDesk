package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// 每個遠端各有設定檔，並行連接不同遠端時不互相覆寫。
type viewerPreferences struct {
	Quality    int  `json:"quality"`
	Scale      int  `json:"scale"`
	Display    int  `json:"display"`
	Fullscreen bool `json:"fullscreen"`
	Control    bool `json:"control"`
	Width      int  `json:"width"`
	Height     int  `json:"height"`
}
type viewerPreferenceStore struct {
	current viewerPreferences // 只在遊戲執行緒讀寫。
	updates chan viewerPreferences
	done    chan struct{}
}

func openViewerPreferences(signal, room string) *viewerPreferenceStore {
	s := &viewerPreferenceStore{current: viewerPreferences{Quality: 1, Scale: 2, Control: true, Width: 1280, Height: 720}, updates: make(chan viewerPreferences, 1), done: make(chan struct{})}
	dir, err := os.UserConfigDir()
	path := ""
	if err == nil {
		key := sha256.Sum256([]byte(signal + "\x00" + room))
		path = filepath.Join(dir, "YourDesk", "viewers", hex.EncodeToString(key[:])+".json")
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			loaded := s.current
			if err := json.Unmarshal(data, &loaded); err != nil {
				slog.Warn("遠端顯示 設定格式錯誤，使用預設值", "error", err)
			} else {
				s.current = loaded
			}
		} else if !os.IsNotExist(readErr) {
			slog.Warn("遠端顯示 設定讀取失敗", "error", readErr)
		}
	} else {
		slog.Warn("遠端顯示 設定目錄無法取得", "error", err)
	}
	if s.current.Quality < 0 || s.current.Quality > 2 {
		s.current.Quality = 1
	}
	if s.current.Scale < 0 || s.current.Scale > 2 {
		s.current.Scale = 2
	}
	if s.current.Display < 0 {
		s.current.Display = 0
	}
	if s.current.Width < 640 || s.current.Width > 16384 {
		s.current.Width = 1280
	}
	if s.current.Height < 270 || s.current.Height > 16384 {
		s.current.Height = 720
	}
	go func() {
		defer close(s.done)
		for value := range s.updates {
			if path == "" {
				continue
			}
			if err := writeViewerPreferences(path, value); err != nil {
				slog.Warn("遠端顯示 設定保存失敗", "error", err)
			}
		}
	}()
	return s
}
func writeViewerPreferences(path string, value viewerPreferences) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".viewer-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}
func (s *viewerPreferenceStore) Save(value viewerPreferences) {
	if value == s.current {
		return
	}
	s.current = value
	select {
	case <-s.updates:
	default:
	}
	s.updates <- value
}
func (s *viewerPreferenceStore) Close() { close(s.updates); <-s.done }

func (g *game) updatePreferences() {
	if g.preferences == nil || g.finalWidth < 1 || nativeFullscreenTransitioning() {
		return
	}
	if g.restorePreferences {
		g.restorePreferences = false
		if g.preferences.current.Fullscreen && !g.isFullscreen() {
			g.toggleFullscreen()
		}
	}
	selected, count, pending := g.displayStatus()
	if g.restoreDisplay && count > 0 && !pending {
		g.restoreDisplay = false
		target := g.preferences.current.Display
		if target < count && target != selected {
			g.selectDisplay(target)
			return
		}
	}
	if nativeFullscreenTransitioning() {
		return
	}
	now := time.Now()
	sampleGeometry := now.Sub(g.preferenceSample) >= 300*time.Millisecond
	value := g.preferences.current
	value.Quality, value.Scale, value.Control = g.quality, int(g.mode), g.controlEnabled
	value.Fullscreen = g.isFullscreen()
	if !value.Fullscreen && sampleGeometry {
		value.Width, value.Height = ebiten.WindowSize()
		g.preferenceSample = now
	}
	if !g.restoreDisplay && !pending && count > 0 {
		value.Display = selected
	}
	g.preferences.Save(value)
}
