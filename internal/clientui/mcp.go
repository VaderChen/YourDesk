package clientui

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"yourdesk/internal/agentremote"
)

// MCP 僅公開具名操作，不轉送任意 API，也不回傳 APP 密碼或完整程序參數。
func (s *server) mcpAPI(ctx context.Context, method, path string, input any) (map[string]any, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data)).WithContext(ctx)
	req.Header.Set("X-YourDesk-Token", s.token)
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	s.api(out, req)
	var result map[string]any
	if err = json.Unmarshal(out.Body.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("無法讀取操作結果")
	}
	if out.Code >= 400 {
		return nil, fmt.Errorf("%v", result["error"])
	}
	return result, nil
}

type mcpConnect struct {
	ID          string `json:"id,omitempty" jsonschema:"已儲存站台 ID；與 room 擇一"`
	Room        string `json:"room,omitempty" jsonschema:"遠端裝置 ID；與 id 擇一"`
	Secret      string `json:"secret,omitempty" jsonschema:"遠端連線密碼，可省略以使用已記住的密碼"`
	Diagnostics bool   `json:"diagnostics,omitempty" jsonschema:"只做背景分析，不開啟遠端顯示；限已儲存站台"`
}
type mcpSession struct {
	Session string `json:"session" jsonschema:"get_status 回傳的 session，例如 viewer:站台ID 或 quick"`
}
type mcpDiagnostic struct {
	ID    string `json:"id"`
	Start bool   `json:"start,omitempty"`
}
type mcpAction struct {
	Session string  `json:"session"`
	Action  string  `json:"action" jsonschema:"screenshot、move、button、key、scroll、text、display"`
	X       float64 `json:"x,omitempty" jsonschema:"畫面相對座標 0～1"`
	Y       float64 `json:"y,omitempty" jsonschema:"畫面相對座標 0～1"`
	Button  int     `json:"button,omitempty" jsonschema:"0 左鍵、1 右鍵、2 中鍵"`
	Down    bool    `json:"down,omitempty" jsonschema:"button 或 key：true 按下，false 放開；按下後必須放開"`
	Key     string  `json:"key,omitempty" jsonschema:"按鍵名稱，例如 a、enter、control、shift"`
	Text    string  `json:"text,omitempty" jsonschema:"UTF-8 文字，最多 16 KB；會取代本機及遠端剪貼簿"`
	Delta   float64 `json:"delta,omitempty"`
	Display int     `json:"display,omitempty" jsonschema:"螢幕索引，從 0 開始"`
}

func (s *server) mcpServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "YourDesk", Version: currentVersion()}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "get_status", Description: "取得本機版本、Host 與遠端顯示子程序、連線階段及佔用提示；不包含密碼。"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		info := map[string]string{}
		for _, k := range []string{"hostname", "room", "platform", "architecture", "version"} {
			info[k] = s.info[k]
		}
		children := []map[string]any{}
		for key, p := range s.children {
			pid := 0
			if p.cmd.Process != nil {
				pid = p.cmd.Process.Pid
			}
			children = append(children, map[string]any{"session": key, "pid": pid, "parentPID": os.Getpid(), "role": p.kind, "room": p.room, "stage": p.stage, "error": p.failureMessage, "diagnostic": p.diagnosticConnection})
		}
		return nil, map[string]any{"info": info, "pid": os.Getpid(), "processes": children, "notice": s.notice, "hostConflict": s.hostConflict}, nil
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "list_sites", Description: "列出已儲存站台，不包含密碼。"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		sites := append([]Site{}, s.library.Sites...)
		return nil, map[string]any{"sites": sites}, nil
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "connect", Description: "使用正常密碼驗證建立遠端連線。回傳僅代表開始連線，請用 get_status 確認 connected。"}, func(ctx context.Context, r *mcp.CallToolRequest, in mcpConnect) (*mcp.CallToolResult, any, error) {
		if (in.ID == "") == (in.Room == "") {
			return nil, nil, fmt.Errorf("id 與 room 必須擇一")
		}
		path := "/api/viewer/start"
		if in.Room != "" {
			if in.Diagnostics {
				return nil, nil, fmt.Errorf("背景分析須使用已儲存站台")
			}
			path = "/api/quick/start"
		}
		out, err := s.mcpAPI(context.WithValue(ctx, mcpConnectContextKey{}, true), "POST", path, in)
		return nil, out, err
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "disconnect", Description: "中斷指定遠端顯示連線；不能停止本機或其他 Host。"}, func(ctx context.Context, r *mcp.CallToolRequest, in mcpSession) (*mcp.CallToolResult, any, error) {
		if in.Session != "quick" && !strings.HasPrefix(in.Session, "viewer:") {
			return nil, nil, fmt.Errorf("僅能中斷遠端顯示")
		}
		out, err := s.mcpAPI(ctx, "POST", "/api/stop", map[string]string{"key": in.Session})
		return nil, out, err
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "get_preferences", Description: "讀取 APP 設定。"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return nil, s.preferences, nil
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "set_preferences", Description: "套用指定設定欄位，未提供的保留。修改 codec 或 directListen 會重新啟動本機 Host 並影響連線。"}, func(ctx context.Context, r *mcp.CallToolRequest, in map[string]any) (*mcp.CallToolResult, any, error) {
		schemaData, _ := json.Marshal(Preferences{})
		var allowed map[string]any
		_ = json.Unmarshal(schemaData, &allowed)
		for k := range in {
			if _, ok := allowed[k]; !ok {
				return nil, nil, fmt.Errorf("未知設定：%s", k)
			}
		}
		out, err := s.mcpAPI(ctx, "PUT", "/api/preferences", in)
		return nil, out, err
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "connection_diagnostics", Description: "取得已連線站台的串流取樣；start=true 重設取樣起點，稍後再讀取。"}, func(ctx context.Context, r *mcp.CallToolRequest, in mcpDiagnostic) (*mcp.CallToolResult, any, error) {
		method := "GET"
		if in.Start {
			method = "POST"
		}
		out, err := s.mcpAPI(ctx, method, "/api/diagnostics?id="+url.QueryEscape(in.ID), nil)
		return nil, out, err
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "remote_action", Description: "操作已連線的遠端：screenshot 回傳 PNG；move/button 使用 0～1 相對座標；key/button 必須成對按下與放開；text 經剪貼簿貼上並取代兩端剪貼簿。display 切換後需重新取畫面。F12 停用控制時不接受輸入。"}, func(ctx context.Context, r *mcp.CallToolRequest, in mcpAction) (*mcp.CallToolResult, any, error) {
		result, err := s.callRemoteAgent(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		if len(result.Image) > 0 {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: result.Image, MIMEType: "image/png"}, &mcp.TextContent{Text: fmt.Sprintf("%dx%d；螢幕 %d / %d；滑鼠使用 0～1 相對座標", result.Width, result.Height, result.Display, result.DisplayCount)}}}, nil, nil
		}
		return nil, result, nil
	})
	return srv
}
func (s *server) callRemoteAgent(ctx context.Context, in mcpAction) (agentremote.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return agentremote.Response{}, err
	}
	id := hex.EncodeToString(idBytes)
	req := agentremote.Request{ID: id, Action: in.Action, X: in.X, Y: in.Y, Button: in.Button, Down: in.Down, Key: in.Key, Text: in.Text, Delta: in.Delta, Display: in.Display, Expires: time.Now().Add(8 * time.Second).UnixMilli()}
	data, err := json.Marshal(map[string]any{"agent": req})
	if err != nil {
		return agentremote.Response{}, err
	}
	if len(data) > 60000 {
		return agentremote.Response{}, fmt.Errorf("操作內容過長")
	}
	ch := make(chan agentremote.Response, 1)
	s.mu.Lock()
	p := s.children[in.Session]
	if p == nil || (p.kind != "viewer" && p.kind != "quick") || p.diagnosticConnection || p.stage != "connected" || p.stdin == nil {
		s.mu.Unlock()
		return agentremote.Response{}, fmt.Errorf("遠端顯示尚未連線或不支援操作")
	}
	if p.agentPending == nil {
		p.agentPending = make(map[string]chan agentremote.Response)
	}
	if len(p.agentPending) >= 8 {
		s.mu.Unlock()
		return agentremote.Response{}, fmt.Errorf("操作佇列已滿")
	}
	if !p.mcpOwned {
		p.mcpOwned = true
		setMCPVisibility(p, s.preferences.MCPOpenDisplay)
	}
	p.agentPending[id] = ch
	_, err = p.stdin.Write(append(data, '\n'))
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(p.agentPending, id); s.mu.Unlock() }()
	if err != nil {
		return agentremote.Response{}, fmt.Errorf("無法傳送操作")
	}
	select {
	case out := <-ch:
		if out.Error != "" {
			return out, fmt.Errorf("%s", out.Error)
		}
		return out, nil
	case <-p.done:
		return agentremote.Response{}, fmt.Errorf("遠端連線已結束")
	case <-ctx.Done():
		return agentremote.Response{}, fmt.Errorf("操作逾時或取消；請重新取得畫面確認結果，勿直接重送")
	}
}
func (s *server) startMCP(ctx context.Context) (func(), error) {
	if os.Getenv("YOURDESK_MCP_DISABLE") == "1" {
		return nil, fmt.Errorf("MCP 已由環境變數停用")
	}
	token := os.Getenv("YOURDESK_MCP_TOKEN")
	tokenPath := filepath.Join(filepath.Dir(s.configPath), "mcp-token")
	if token == "" {
		data, err := os.ReadFile(tokenPath)
		if os.IsNotExist(err) {
			b := make([]byte, 32)
			if _, err = rand.Read(b); err != nil {
				return nil, err
			}
			data = []byte(hex.EncodeToString(b))
			f, e := os.OpenFile(tokenPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return nil, e
			}
			_, err = f.Write(data)
			closeErr := f.Close()
			if err != nil {
				return nil, err
			}
			if closeErr != nil {
				return nil, closeErr
			}
		} else if err != nil {
			return nil, err
		}
		token = strings.TrimSpace(string(data))
	}
	if len(token) < 32 {
		return nil, fmt.Errorf("MCP Token 至少須有 32 個字元")
	}
	listener, err := net.Listen("tcp4", "0.0.0.0:12345")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	srv := s.mcpServer()
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	slots := make(chan struct{}, 8)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		allowed, tokenRequired := s.mcpRemoteAccess(r.RemoteAddr)
		if !allowed {
			http.Error(w, "IP not allowed", http.StatusForbidden)
			return
		}
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		host, port, err := net.SplitHostPort(r.Host)
		if err != nil || port != "12345" || (host != "localhost" && net.ParseIP(host) == nil) {
			http.Error(w, "Invalid host", 403)
			return
		}
		if r.Header.Get("Origin") != "" {
			http.Error(w, "Browser origins are not allowed", 403)
			return
		}
		if tokenRequired && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", 401)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "Busy", 429)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 65536)
		transport.ServeHTTP(w, r)
	})
	httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() { _ = httpServer.Serve(listener) }()
	go func() { <-ctx.Done(); _ = httpServer.Close() }()
	fmt.Println("MCP：http://0.0.0.0:12345/mcp；Token 檔案：", tokenPath)
	return func() { cancel(); _ = httpServer.Close() }, nil
}

// 僅信任 TCP 對端位址，不採用可由呼叫者偽造的轉送標頭。
func (s *server) mcpRemoteAccess(remoteAddr string) (allowed, tokenRequired bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.preferences.MCPWhitelistEnabled {
		return true, true
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false, true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false, true
	}
	for _, address := range s.preferences.MCPWhitelist {
		if ip.Equal(net.ParseIP(address)) {
			return true, false
		}
	}
	return false, true
}
