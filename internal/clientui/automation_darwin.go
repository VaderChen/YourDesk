//go:build darwin

package clientui

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Siri 的本機能力憑證獨立於 Web UI/MCP，只能操作固定動作。
func (s *server) installAutomation(mux *http.ServeMux) (func(), error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(b)
	path := filepath.Join(filepath.Dir(s.configPath), "siri-bridge.json")
	data, _ := json.Marshal(map[string]any{"url": s.origin + "/automation", "token": token, "pid": os.Getpid()})
	f, err := os.CreateTemp(filepath.Dir(path), ".siri-bridge-*")
	if err != nil {
		return nil, err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	if err = os.Rename(temp, path); err != nil {
		return nil, err
	}
	mux.HandleFunc("/automation", s.automationHandler(token))
	return func() { os.Remove(path) }, nil
}

type siriRequest struct {
	Action string `json:"action"`
	ID     string `json:"id,omitempty"`
}
type siriSite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *server) automationHandler(token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 || r.Header.Get("Origin") != "" {
			http.Error(w, "未授權", 403)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "只接受 POST", 405)
			return
		}
		var in siriRequest
		if err := decode(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		switch in.Action {
		case "open", "show-site":
			query := ""
			if in.Action == "show-site" {
				s.mu.Lock()
				for _, site := range s.library.Sites {
					if site.ID == in.ID {
						query = site.Room
						break
					}
				}
				s.mu.Unlock()
				if query == "" {
					fail(w, fmt.Errorf("找不到已儲存站台"))
					return
				}
			}
			select {
			case s.automationShow <- query:
			default:
			}
			respond(w, 200, map[string]string{"message": "已開啟 YourDesk"})
		case "sites":
			s.mu.Lock()
			sites := []siriSite{}
			for _, site := range s.library.Sites {
				sites = append(sites, siriSite{site.ID, site.Name})
			}
			s.mu.Unlock()
			sort.Slice(sites, func(i, j int) bool { return sites[i].Name < sites[j].Name })
			respond(w, 200, map[string]any{"sites": sites})
		case "connect":
			s.mu.Lock()
			found := false
			remembered := false
			for _, site := range s.library.Sites {
				if site.ID == in.ID {
					found = true
					remembered = s.remembered(site.Signal, site.Room) != ""
					break
				}
			}
			s.mu.Unlock()
			if !found {
				fail(w, fmt.Errorf("找不到已儲存站台"))
				return
			}
			if !remembered {
				select {
				case s.automationShow <- "":
				default:
				}
				fail(w, fmt.Errorf("請先在 YourDesk 連線此站台並記住密碼"))
				return
			}
			if _, err := s.mcpAPI(r.Context(), "POST", "/api/viewer/start", map[string]string{"id": in.ID}); err != nil {
				fail(w, err)
				return
			}
			respond(w, 200, map[string]string{"message": "已開始連線，請稍後查詢連線狀態"})
		case "status", "disconnect":
			s.mu.Lock()
			keys := []string{}
			states := []string{}
			for key, p := range s.children {
				if key != "quick" && !strings.HasPrefix(key, "viewer:") {
					continue
				}
				if in.ID != "" && key != "viewer:"+in.ID {
					continue
				}
				name := p.room
				for _, site := range s.library.Sites {
					if site.ID == p.siteID {
						name = site.Name
						break
					}
				}
				keys = append(keys, key)
				stage := map[string]string{"connected": "已連線", "connecting": "連線中", "disconnected": "已斷線", "failed": "連線失敗", "auth-required": "等待密碼"}[p.stage]
				if stage == "" {
					stage = "連線準備中"
				}
				states = append(states, name+"："+stage)
			}
			s.mu.Unlock()
			if in.Action == "disconnect" {
				if in.ID == "" && len(keys) > 1 {
					fail(w, fmt.Errorf("目前有多個連線，請指定電腦"))
					return
				}
				for _, key := range keys {
					if _, err := s.mcpAPI(r.Context(), "POST", "/api/stop", map[string]string{"key": key}); err != nil {
						fail(w, err)
						return
					}
				}
				message := "已中斷連線"
				if len(keys) == 0 {
					message = "目前沒有符合的連線"
				}
				respond(w, 200, map[string]string{"message": message})
				return
			}
			sort.Strings(states)
			message := "目前沒有對外連線"
			if len(states) > 0 {
				message = strings.Join(states, "；")
			}
			respond(w, 200, map[string]string{"message": message})
		default:
			fail(w, fmt.Errorf("不支援的 Siri 操作"))
		}
	}
}
