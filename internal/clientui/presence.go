package clientui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
)

func (s *server) servePresence(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	sites := append([]Site(nil), s.library.Sites...)
	s.mu.Unlock()
	states := make(map[string]*bool, len(sites))
	groups := make(map[string][]Site)
	for _, site := range sites {
		states[site.ID] = nil
		if _, direct := signaling.DirectAddress(site.Room); direct {
			continue
		}
		groups[site.Signal] = append(groups[site.Signal], site)
	}
	var mu sync.Mutex
	var pending sync.WaitGroup
	slots := make(chan struct{}, 4)
	client, err := security.TLSHTTPClient(3 * time.Second)
	if err != nil {
		respond(w, 200, states)
		return
	}
	// 本次查詢使用獨立連線池，所有工作結束後釋放閒置連線。
	defer client.CloseIdleConnections()
	for signal, group := range groups {
		pending.Add(1)
		go func(signal string, group []Site) {
			defer pending.Done()
			select {
			case slots <- struct{}{}:
			case <-r.Context().Done():
				return
			}
			defer func() { <-slots }()
			address, err := url.Parse(signal)
			if err != nil {
				return
			}
			switch address.Scheme {
			case "ws":
				address.Scheme = "https"
			case "wss":
				address.Scheme = "https"
			default:
				return
			}
			address.Path = "/presence"
			address.RawPath = ""
			address.RawQuery = ""
			address.Fragment = ""
			for start := 0; start < len(group); start += 256 {
				batch := group[start:min(start+256, len(group))]
				rooms := make([]string, 0, len(batch))
				for _, site := range batch {
					rooms = append(rooms, site.Room)
				}
				body, _ := json.Marshal(map[string]any{"rooms": rooms})
				request, err := http.NewRequestWithContext(r.Context(), "POST", address.String(), bytes.NewReader(body))
				if err != nil {
					return
				}
				request.Header.Set("Content-Type", "application/json")
				response, err := client.Do(request)
				if err != nil {
					return
				}
				var result map[string]bool
				if response.StatusCode == 200 {
					err = json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&result)
				}
				response.Body.Close()
				if response.StatusCode != 200 || err != nil {
					return
				}
				mu.Lock()
				for _, site := range batch {
					if value, ok := result[site.Room]; ok {
						online := value
						states[site.ID] = &online
					}
				}
				mu.Unlock()
			}
		}(signal, group)
	}
	pending.Wait()
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, states)
}
