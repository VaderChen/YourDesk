package clipboard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Progress 僅提供傳輸中繼資料，不將文字、圖片內容或完整檔案路徑送入 UI。
type Progress struct {
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Kind      string `json:"kind"`
	Bytes     int64  `json:"bytes"`
	Total     int64  `json:"total"`
	Files     int    `json:"files"`
	Stage     string `json:"stage"`
}
type transferProgress struct {
	Progress
	started  time.Time
	finished time.Time
	shown    bool
}

func (s *Sync) startProgress(meta packet, direction string) {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	for key, p := range s.progress {
		if p.Direction == direction && !p.finished.IsZero() {
			delete(s.progress, key)
		}
	}
	s.progress[direction+meta.ID] = &transferProgress{Progress: Progress{ID: meta.ID, Direction: direction, Kind: meta.Kind, Total: meta.Size, Files: len(meta.Files), Stage: "transferring"}, started: time.Now()}
}
func (s *Sync) updateProgress(id, direction string, bytes int64, stage string) {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if p := s.progress[direction+id]; p != nil {
		p.Bytes = bytes
		p.Stage = stage
	}
}
func (s *Sync) finishProgress(id, direction string, ok bool) {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if p := s.progress[direction+id]; p != nil {
		p.finished = time.Now()
		p.Stage = "failed"
		if ok {
			p.Stage = "done"
			p.Bytes = p.Total
		}
	}
}

// 遠端顯示 直接接收狀態；未註冊 UI 的程序維持原有事件輸出。
func (s *Sync) SetProgressHandler(handler func([]Progress)) {
	s.progressMu.Lock()
	s.progressHandler = handler
	s.progressMu.Unlock()
}
func (s *Sync) deliverProgress(values []Progress, event string) {
	s.progressMu.Lock()
	handler := s.progressHandler
	s.progressMu.Unlock()
	if handler != nil {
		handler(values)
	} else {
		fmt.Fprintln(os.Stdout, event)
	}
}
func (s *Sync) reportProgress(ctx context.Context) {
	emit := func(values []Progress) string {
		data, _ := json.Marshal(struct {
			Event     string     `json:"event"`
			Transfers []Progress `json:"transfers"`
		}{"clipboard-progress", values})
		return "YOURDESK_UI_EVENT " + string(data)
	}
	last := emit([]Progress{})
	defer func() {
		if last != emit([]Progress{}) {
			s.deliverProgress([]Progress{}, emit([]Progress{}))
		}
	}()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		now := time.Now()
		values := []Progress{}
		s.progressMu.Lock()
		for key, p := range s.progress {
			if !p.finished.IsZero() && !p.shown && p.finished.Sub(p.started) <= 2*time.Second {
				delete(s.progress, key)
				continue
			}
			if !p.finished.IsZero() && p.Stage == "done" && now.Sub(p.finished) > 2*time.Second {
				delete(s.progress, key)
				continue
			}
			if now.Sub(p.started) > 2*time.Second {
				p.shown = true
				values = append(values, p.Progress)
			}
		}
		s.progressMu.Unlock()
		sort.Slice(values, func(i, j int) bool { return values[i].Direction+values[i].ID < values[j].Direction+values[j].ID })
		next := emit(values)
		if next != last {
			s.deliverProgress(values, next)
			last = next
		}
	}
}
