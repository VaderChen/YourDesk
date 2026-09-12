package clientui

import "net/http"

// 舊啟動設定 API 共用登入前服務流程，避免建立登入後的使用者啟動項目。
func (s *server) handleAutostart(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/autostart" {
		return false
	}
	if r.Method != "GET" && r.Method != "PUT" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return true
	}
	request := r.Clone(r.Context())
	request.URL.Path = "/api/prelogin"
	if request.Method == "PUT" {
		request.Method = "POST"
	}
	return s.handlePrelogin(w, request)
}
