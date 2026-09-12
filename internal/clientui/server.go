// Package clientui 提供僅在本機開放的 HTML 管理介面。
package clientui

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"yourdesk/internal/authlog"
	"yourdesk/internal/childprocess"
	"yourdesk/internal/clipboard"

	"yourdesk/internal/agentremote"
	"yourdesk/internal/deviceid"
	"yourdesk/internal/diagnostics"
	"yourdesk/internal/frameinterp"
	"yourdesk/internal/hostguard"
	"yourdesk/internal/prelogin"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
	"yourdesk/internal/superres"
	"yourdesk/internal/video"
)

//go:embed web/*
var assets embed.FS

type Options struct {
	Signal, Room, Secret, DirectListen string
	HostArgs                           []string
}
type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Site struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Room   string `json:"room"`
	Signal string `json:"signal"`
	Group  string `json:"group"`
	Note   string `json:"note"`
}
type Library struct {
	Groups []Group `json:"groups"`
	Sites  []Site  `json:"sites"`
}
type process struct {
	incomingGeneration uint64
	incomingConnected  bool

	mcpOwned                     bool
	mcpVisible                   bool
	agentPending                 map[string]chan agentremote.Response
	passwordProof                string
	diagnostic                   *diagnosticRun
	diagnosticConnection         bool
	terminalInstance             string
	terminalOwner                *process
	terminalConnection           bool
	transfers                    []clipboard.Progress
	credentialKey, pendingSecret string
	remember                     bool
	stdin                        io.WriteCloser
	room                         string
	stage                        string
	failureMessage               string
	authMessage                  string
	authRequired                 bool
	cmd                          *exec.Cmd
	done                         chan struct{}
	kind, siteID                 string
}
type Preferences struct {
	SelectedGroup           string   `json:"selectedGroup"`
	MCPWhitelistEnabled     bool     `json:"mcpWhitelistEnabled"`
	MCPWhitelist            []string `json:"mcpWhitelist"`
	MCPOpenDisplay          bool     `json:"mcpOpenDisplay"`
	MCPEnabled              bool     `json:"mcpEnabled"`
	CloseWindowOnDisconnect bool     `json:"closeWindowOnDisconnect"`
	FitWindow               bool     `json:"fitWindow"`
	SourceFPSLimit          int      `json:"sourceFPSLimit"`
	BitrateLimitMbps        int      `json:"bitrateLimitMbps"`
	EnhancementStrategy     string   `json:"enhancementStrategy"`
	EnhancementBitrateMbps  int      `json:"enhancementBitrateMbps"`
	KeyframeInterval        int      `json:"keyframeInterval"`
	InterpolationMethod     string   `json:"interpolationMethod"`
	Interpolation           bool     `json:"interpolation"`
	CoreMLModel             string   `json:"coreMLModel"`
	SuperResolution         string   `json:"superResolution"`
	ImageEnhancement        bool     `json:"imageEnhancement"`
	DisableKeyMapping       bool     `json:"disableKeyMapping"`
	DisableHints            bool     `json:"disableHints"`
	TailcatEnabled          bool     `json:"tailcatEnabled"`
	DirectListen            bool     `json:"directListen"`
	Codec                   string   `json:"codec"`
	Language                string   `json:"language"`
	Theme                   string   `json:"theme"`
}

type server struct {
	incomingActive  bool
	preloginBusy    bool
	preloginMessage string

	mcpStop                               func()
	updater                               *updateManager
	transferNotice                        chan struct{}
	connected                             chan struct{}
	closeApplication                      context.CancelFunc
	viewerClosed                          chan struct{}
	preferences                           Preferences
	mu                                    sync.Mutex
	options                               Options
	library                               Library
	configPath, executable, token, origin string
	info                                  map[string]string
	children                              map[string]*process
	notice                                string
	hostConflict                          *hostguard.Owner
	hostConflictPreflight                 bool
}

func Run(ctx context.Context, options Options) error {
	superres.StartProbe()
	if err := windowAvailable(); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	configDir = filepath.Join(configDir, "YourDesk")
	if err = os.MkdirAll(configDir, 0700); err != nil {
		return err
	}
	releaseInstance, err := lockInstance(filepath.Join(configDir, "client-ui.lock"))
	if err != nil {
		return err
	}
	defer releaseInstance()
	s := &server{transferNotice: make(chan struct{}, 1), options: options, executable: executable, configPath: filepath.Join(configDir, "sites.json"),
		viewerClosed: make(chan struct{}, 1), children: make(map[string]*process), library: Library{Groups: []Group{}, Sites: []Site{}}}
	if data, err := os.ReadFile(s.configPath); err == nil {
		if err = json.Unmarshal(data, &s.library); err != nil {
			return fmt.Errorf("站台設定無法讀取，請保留 %s：%w", s.configPath, err)
		}
		if err = validate(s.library); err != nil {
			return fmt.Errorf("站台設定無效：%w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.preferences = Preferences{SelectedGroup: "*", MCPWhitelistEnabled: true, MCPWhitelist: []string{"127.0.0.1"}, ImageEnhancement: false, Interpolation: false, BitrateLimitMbps: 12, SourceFPSLimit: 20, KeyframeInterval: 10, Language: "auto", Theme: "default", DirectListen: options.DirectListen != ""}
	if data, readErr := os.ReadFile(filepath.Join(configDir, "preferences.json")); readErr == nil {
		if err := json.Unmarshal(data, &s.preferences); err != nil {
			return fmt.Errorf("無法讀取介面設定：%w", err)
		}
		if s.preferences.BitrateLimitMbps < 1 || s.preferences.BitrateLimitMbps > 64 {
			s.preferences.BitrateLimitMbps = 12
		}
		if s.preferences.SourceFPSLimit < 5 || s.preferences.SourceFPSLimit > 60 {
			s.preferences.SourceFPSLimit = 20
		}
		if err := s.preferences.validate(); err != nil {
			return err
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if s.preferences.Codec == "" {
		s.preferences.Codec = "auto"
	}
	hostname, _ := os.Hostname()
	room := options.Room
	source := "手動指定"
	if room == "" {
		identity, err := deviceid.Current()
		if err != nil {
			return err
		}
		room, source = identity.UID, string(identity.Source)
	}
	authlog.Event("identity", map[string]any{"room": room, "source": source})
	s.info = map[string]string{"hostname": hostname, "room": room, "source": source, "platform": runtime.GOOS,
		"architecture": runtime.GOARCH, "configPath": s.configPath, "version": currentVersion()}
	secret, needsChange, err := security.LocalSecret("")
	if err != nil {
		return err
	}
	s.info["passwordNeedsChange"] = fmt.Sprint(needsChange)
	s.options.Secret = secret
	s.info["secret"] = secret
	// 明確傳入畫面上顯示的配對資訊，避免每次啟動產生不同密鑰。
	s.options.HostArgs = append(append([]string{}, options.HostArgs...), "-room", room)
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return err
	}
	s.token = hex.EncodeToString(token)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.origin = "http://" + listener.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/", s.api)
	web, _ := fs.Sub(assets, "web")
	mux.Handle("/", http.FileServer(http.FS(web)))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != listener.Addr().String() {
			http.Error(w, "無效的主機", 403)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
	httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	windowContext, closeWindow := context.WithCancel(ctx)
	defer closeWindow()
	s.closeApplication = closeWindow
	s.updater = newUpdateManager(windowContext, filepath.Dir(s.configPath))
	finished := make(chan error, 1)
	go func() {
		finished <- httpServer.Serve(listener)
		closeWindow()
	}()
	fmt.Println("YourDesk Client：正在開啟桌面視窗。")
	fmt.Println("站台設定：", s.configPath)
	fmt.Println("關閉視窗只隱藏介面；使用 Tray 的「關閉程式」或 Ctrl-C 結束 Client 與 遠端顯示。")
	s.connected = make(chan struct{}, 8)
	hostSupervisorDone := make(chan struct{})
	go func() {
		defer close(hostSupervisorDone)
		s.keepHostRunning(windowContext)
	}()
	updaterDone := make(chan struct{})
	go func() { defer close(updaterDone); s.updater.run() }()
	s.mu.Lock()
	if s.preferences.MCPEnabled {
		stop, err := s.startMCP(windowContext)
		if err != nil {
			slog.Warn("MCP 啟動失敗", "error", err)
			s.notice = "MCP 無法啟動，請確認 12345 連接埠。"
			s.preferences.MCPEnabled = false
		} else {
			s.mcpStop = stop
		}
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.mcpStop != nil {
			s.mcpStop()
			s.mcpStop = nil
		}
	}()
	err = runWindow(windowContext, s.origin+"/#"+s.token, s.connected, s.updater.notify, s.transferNotice, s.viewerClosed, s)
	closeWindow()
	<-hostSupervisorDone
	<-updaterDone

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdown)
	s.updater.downloads.Wait()
	s.mu.Lock()
	var children []*process
	for _, p := range s.children {
		children = append(children, p)
		_ = p.cmd.Process.Kill()
	}
	s.mu.Unlock()
	for _, p := range children {
		<-p.done
	}
	serverErr := <-finished
	if err != nil {
		return err
	}
	if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
		return serverErr
	}
	return nil
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, err error) { respond(w, 400, map[string]string{"error": err.Error()}) }
func decode(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return fmt.Errorf("資料格式錯誤：%w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("只能傳送一份 JSON 資料")
	}
	return nil
}
func (s *server) api(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-YourDesk-Token")), []byte(s.token)) != 1 {
		respond(w, 403, map[string]string{"error": "請使用 Client 啟動時提供的完整網址開啟介面"})
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != s.origin {
		http.Error(w, "無效來源", 403)
		return
	}
	if r.URL.Path == "/api/presence" && r.Method == "GET" {
		s.servePresence(w, r)
		return
	}
	if r.URL.Path == "/api/incoming/disconnect" && r.Method == "POST" {
		if err := s.disconnectIncoming(); err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.URL.Path == "/api/terminal/preferences" && r.Method == "GET" {
		s.mu.Lock()
		closeWindow := s.preferences.CloseWindowOnDisconnect
		s.mu.Unlock()
		respond(w, 200, map[string]bool{"closeWindowOnDisconnect": closeWindow})
		return
	}
	if r.URL.Path == "/api/terminal/window" && r.Method == "POST" {
		s.openTerminalWindow(w, r)
		return
	}
	if r.URL.Path == "/api/terminal" && r.Method == "POST" {
		s.terminalAction(w, r)
		return
	}
	if s.handleAutostart(w, r) {
		return
	}
	if s.handlePrelogin(w, r) {
		return
	}
	if s.handleUpdates(w, r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.URL.Path == "/api/host-conflict/stop" && r.Method == "POST":
		var owner hostguard.Owner
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&owner) != nil || s.hostConflict == nil || *s.hostConflict != owner {
			fail(w, errors.New("程序資訊已變更，請重新確認"))
			return
		}
		var stopErr error
		if s.hostConflictPreflight {
			for _, child := range s.children {
				if child.cmd.Process != nil && child.cmd.Process.Pid == owner.PID {
					fail(w, errors.New("此程序由目前 APP 管理，未停止"))
					return
				}
			}
			stopErr = hostguard.StopCandidate(owner)
		} else {
			stopErr = hostguard.StopOwner(s.info["room"], owner)
		}
		if err := stopErr; err != nil {
			fail(w, err)
			return
		}
		s.hostConflict = nil
		s.notice = ""
		respond(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/stream-auto" && r.Method == "POST":
		s.autoStream(w, r)
	case r.URL.Path == "/api/diagnostics" && (r.Method == "GET" || r.Method == "POST" || r.Method == "DELETE"):
		s.connectionDiagnostics(w, r)
	case r.URL.Path == "/api/transfers" && r.Method == "GET":
		respond(w, 200, s.transferSnapshot())
	case r.URL.Path == "/api/state" && r.Method == "GET":
		running := make(map[string]string)
		sessions := make(map[string]map[string]string)
		for key, p := range s.children {
			running[key] = p.siteID
			if p.kind == "viewer" || p.kind == "quick" {
				sessions[key] = map[string]string{"stage": p.stage, "error": p.failureMessage}
			}
		}
		var quick any
		if p := s.children["quick"]; p != nil {
			quick = map[string]any{"room": p.room, "stage": p.stage, "authRequired": p.authRequired, "message": p.authMessage}
		}
		respond(w, 200, map[string]any{"incomingConnected": s.incomingActive, "prelogin": s.preloginState(), "info": s.info, "library": publicLibrary(s.library), "running": running, "sessions": sessions, "notice": s.notice, "hostConflict": s.hostConflict, "quick": quick, "passwordPrompt": s.passwordPrompt(), "preferences": s.preferences, "updates": s.updater.snapshot(), "hardwareJPEG": video.ProbeHardwareJPEG().Encode, "superResolutionCapabilities": superres.Capabilities(), "coreMLModels": superres.Models(), "appleInterpolationSupported": frameinterp.AppleSupported(), "videoCapabilities": video.CachedIntraCapabilities()})
	case r.URL.Path == "/api/local-password" && r.Method == "PUT":
		if s.preloginBusy || prelogin.Status().Enabled {
			fail(w, errors.New("請先停用未登入開機，再修改配對密碼。"))
			return
		}
		var request struct {
			Secret string `json:"secret"`
		}
		if err := decode(w, r, &request); err != nil {
			fail(w, err)
			return
		}
		if _, err := security.DecodeSecret(request.Secret); err != nil {
			fail(w, err)
			return
		}
		if request.Secret == s.options.Secret {
			fail(w, errors.New("請設定不同於目前密碼的新密碼"))
			return
		}
		if err := security.SaveLocalSecret(request.Secret, false); err != nil {
			fail(w, err)
			return
		}
		s.options.Secret = request.Secret
		s.info["secret"] = request.Secret
		s.info["passwordNeedsChange"] = "false"
		if host := s.children["host"]; host != nil {
			_ = host.cmd.Process.Kill()
		}
		respond(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/preferences" && r.Method == "PUT":
		preferences := s.preferences
		preferences.MCPWhitelist = append([]string(nil), s.preferences.MCPWhitelist...)
		if err := decode(w, r, &preferences); err != nil {
			fail(w, err)
			return
		}
		if err := preferences.validate(); err != nil {
			fail(w, err)
			return
		}
		if (s.preloginBusy || prelogin.Status().Enabled) && (preferences.Codec != s.preferences.Codec || preferences.DirectListen != s.preferences.DirectListen || preferences.TailcatEnabled != s.preferences.TailcatEnabled) {
			fail(w, errors.New("請先停用未登入開機，再修改 Host 的影像傳輸、IP 直連或 Tailcat 模式。"))
			return
		}
		startedMCP := false
		if preferences.MCPEnabled && s.mcpStop == nil {
			stop, err := s.startMCP(s.updater.ctx)
			if err != nil {
				fail(w, fmt.Errorf("MCP 無法啟動：%w", err))
				return
			}
			s.mcpStop = stop
			startedMCP = true
		}
		if err := saveJSON(filepath.Join(filepath.Dir(s.configPath), "preferences.json"), preferences); err != nil {
			if startedMCP {
				s.mcpStop()
				s.mcpStop = nil
			}
			fail(w, err)
			return
		}
		if !preferences.MCPEnabled && s.mcpStop != nil {
			s.mcpStop()
			s.mcpStop = nil
		}
		hostChanged := s.preferences.Codec != preferences.Codec || s.preferences.DirectListen != preferences.DirectListen || s.preferences.TailcatEnabled != preferences.TailcatEnabled
		s.preferences = preferences
		if hostChanged {
			if host := s.children["host"]; host != nil {
				_ = host.cmd.Process.Kill()
			}
		}
		respond(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/library/export" && r.Method == "POST":
		path, err := exportLibrary(s.library)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]string{"path": path})
	case r.URL.Path == "/api/library" && r.Method == "PUT":
		var library Library
		if err := decode(w, r, &library); err != nil {
			fail(w, err)
			return
		}
		for i := range library.Sites {
			library.Sites[i].Signal = s.options.Signal
			for _, old := range s.library.Sites {
				if old.ID == library.Sites[i].ID {
					library.Sites[i].Signal = old.Signal
					break
				}
			}
		}
		if err := validate(library); err != nil {
			fail(w, err)
			return
		}
		// 儲存時保留執行中站台的識別，避免失去停止連線的操作入口。
		for _, p := range s.children {
			if p.kind != "viewer" {
				continue
			}
			found := false
			for _, site := range library.Sites {
				if site.ID == p.siteID {
					found = true
				}
			}
			if !found {
				fail(w, errors.New("請先關閉此站台的 遠端顯示，再刪除站台"))
				return
			}
		}
		if err := s.save(library); err != nil {
			fail(w, err)
			return
		}
		s.library = library
		respond(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/remembered" && r.Method == "POST":
		var request struct {
			ID string `json:"id"`
		}
		if err := decode(w, r, &request); err != nil {
			fail(w, err)
			return
		}
		remembered := false
		for _, site := range s.library.Sites {
			if site.ID == request.ID {
				remembered = s.remembered(site.Signal, site.Room) != ""
				break
			}
		}
		respond(w, 200, map[string]bool{"remembered": remembered})
	case r.URL.Path == "/api/quick/start" && r.Method == "POST":
		var request struct {
			Terminal bool   `json:"terminal"`
			Room     string `json:"room"`
			Secret   string `json:"secret"`
			Remember bool   `json:"remember"`
		}
		if err := decode(w, r, &request); err != nil {
			fail(w, err)
			return
		}
		room := strings.TrimSpace(request.Room)
		if room == "" || len(room) > 200 || strings.ContainsAny(room, "\r\n\t") {
			fail(w, errors.New("請貼上有效的裝置 ID"))
			return
		}
		secret := request.Secret
		if secret == "" {
			secret = s.remembered(s.options.Signal, room)
			request.Remember = secret != ""
		}
		if _, err := security.DecodeSecret(secret); err != nil {
			fail(w, errors.New("請先輸入有效的連線密碼"))
			return
		}
		name := "YourDesk"
		for _, site := range s.library.Sites {
			if site.Room == room && site.Signal == s.options.Signal {
				name = site.Name
				break
			}
		}
		args := []string{"-signal", s.options.Signal, "-room", room, "-name", name, "-secret-stdin", "-interactive-auth"}
		if s.preferences.TailcatEnabled {
			args = append(args, "-transport", "tailcat")
		}
		if r.Context().Value(mcpConnectContextKey{}) == true {
			args = append(args, "-mcp-managed")
			if !s.preferences.MCPOpenDisplay {
				args = append(args, "-mcp-hidden")
			}
		}
		if request.Terminal {
			args = append(args, "-terminal")
		}
		if err := s.start("quick", "", s.viewerBinary(), args); err != nil {
			fail(w, err)
			return
		}
		if err := sendProcessSecret(s.children["quick"], secret); err != nil {
			_ = s.children["quick"].cmd.Process.Kill()
			fail(w, err)
			return
		}
		s.children["quick"].terminalConnection = request.Terminal
		s.children["quick"].credentialKey = credentialID(s.options.Signal, room)
		s.children["quick"].pendingSecret = secret
		s.children["quick"].remember = request.Remember
		s.children["quick"].room = room
		s.children["quick"].stage = "waiting"
		respond(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/quick/password" && r.Method == "POST":
		var request struct {
			Secret   string `json:"secret"`
			Key      string `json:"key"`
			Remember bool   `json:"remember"`
		}
		if err := decode(w, r, &request); err != nil {
			fail(w, err)
			return
		}
		if len(request.Secret) > 1000 {
			fail(w, errors.New("密碼過長"))
			return
		}
		if _, err := security.DecodeSecret(request.Secret); err != nil {
			fail(w, err)
			return
		}
		key := request.Key
		if key == "" {
			key = "quick"
		}
		p := s.children[key]
		if p == nil || !p.authRequired || p.stdin == nil {
			fail(w, errors.New("這次連線已不在等待密碼，請重新連線"))
			return
		}
		if err := json.NewEncoder(p.stdin).Encode(request); err != nil {
			fail(w, errors.New("無法傳送密碼，請重新連線"))
			return
		}
		p.pendingSecret = request.Secret
		p.remember = request.Remember
		p.authRequired = false
		p.stage = "authenticating"
		respond(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/viewer/start" && r.Method == "POST":
		var request struct {
			Diagnostics bool   `json:"diagnostics"`
			Terminal    bool   `json:"terminal"`
			ID          string `json:"id"`
			Secret      string `json:"secret"`
			Remember    bool   `json:"remember"`
		}
		if err := decode(w, r, &request); err != nil {
			fail(w, err)
			return
		}
		if request.Terminal && request.Diagnostics {
			fail(w, errors.New("命令列不支援畫面偵測"))
			return
		}
		var selected *Site
		for i := range s.library.Sites {
			if s.library.Sites[i].ID == request.ID {
				selected = &s.library.Sites[i]
				break
			}
		}
		if selected == nil {
			fail(w, errors.New("找不到站台"))
			return
		}
		if request.Secret == "" {
			request.Secret = s.remembered(selected.Signal, selected.Room)
			request.Remember = request.Secret != ""
		}
		if _, err := security.DecodeSecret(request.Secret); err != nil {
			fail(w, err)
			return
		}
		binary := s.viewerBinary()
		args := []string{"-signal", selected.Signal, "-room", selected.Room, "-name", selected.Name, "-secret-stdin", "-interactive-auth"}
		if s.preferences.TailcatEnabled {
			args = append(args, "-transport", "tailcat")
		}
		if r.Context().Value(mcpConnectContextKey{}) == true && !request.Diagnostics {
			args = append(args, "-mcp-managed")
			if !s.preferences.MCPOpenDisplay {
				args = append(args, "-mcp-hidden")
			}
		}
		if request.Terminal {
			args = append(args, "-terminal")
		}
		if request.Diagnostics {
			args = append(args, "-diagnostic")
		}
		if err := s.start("viewer", selected.ID, binary, args); err != nil {
			fail(w, err)
			return
		}
		p := s.children["viewer:"+selected.ID]
		if err := sendProcessSecret(p, request.Secret); err != nil {
			_ = p.cmd.Process.Kill()
			fail(w, err)
			return
		}
		p.diagnosticConnection = request.Diagnostics
		p.terminalConnection = request.Terminal
		p.room = selected.Room
		p.credentialKey = credentialID(selected.Signal, selected.Room)
		p.pendingSecret = request.Secret
		p.remember = request.Remember
		respond(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/stop" && r.Method == "POST":
		var request struct {
			Key string `json:"key"`
		}
		if err := decode(w, r, &request); err != nil {
			fail(w, err)
			return
		}
		if request.Key == "host" {
			fail(w, errors.New("本機 Client 隨 APP 持續執行，無法單獨停止"))
			return
		}
		if p := s.children[request.Key]; p != nil {
			// 遠端顯示 的既有 UI 迴圈不因 SIGTERM 結束；結束整個子程序才能釋放連線。
			if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				fail(w, err)
				return
			}
		}
		respond(w, 200, map[string]bool{"ok": true})
	default:
		respond(w, 404, map[string]string{"error": "找不到操作"})
	}
}
func validSignal(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "ws" || u.Scheme == "wss" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func validate(l Library) error {
	if len(l.Groups) > 200 || len(l.Sites) > 2000 {
		return errors.New("最多支援 200 個群組與 2000 個站台")
	}
	groups := map[string]bool{"": true}
	for _, g := range l.Groups {
		if strings.TrimSpace(g.ID) == "" || len(g.ID) > 100 || strings.TrimSpace(g.Name) == "" || len(g.Name) > 200 || groups[g.ID] {
			return errors.New("群組名稱或識別碼無效或重複")
		}
		groups[g.ID] = true
	}
	ids := map[string]bool{}
	for _, site := range l.Sites {
		if strings.TrimSpace(site.ID) == "" || len(site.ID) > 100 || ids[site.ID] || strings.TrimSpace(site.Name) == "" || len(site.Name) > 200 || strings.TrimSpace(site.Room) == "" || len(site.Room) > 200 || len(site.Note) > 2000 || len(site.Signal) > 2048 || !validSignal(site.Signal) || !groups[site.Group] {
			return errors.New("站台資料無效：請確認名稱、ROOM、配對服務及群組")
		}
		ids[site.ID] = true
	}
	return nil
}
func (s *server) save(l Library) error { return saveJSON(s.configPath, l) }

func saveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "yourdesk-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
func (s *server) start(kind, siteID, binary string, args []string) error {
	key := kind
	if siteID != "" {
		key += ":" + siteID
	}
	if s.children[key] != nil {
		return errors.New("此程序已在執行")
	}
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("找不到執行檔，請重新執行 runUITest.command：%w", err)
	}
	_, mcpConnection := contextMCP(args)
	cmd := childprocess.Command(binary, args...)
	// 診斷輸出留在啟動終端，不把配對密碼寫入站台設定。
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	p := &process{terminalInstance: rand.Text(), mcpOwned: mcpConnection, mcpVisible: mcpConnection && s.preferences.MCPOpenDisplay, cmd: cmd, done: make(chan struct{}), kind: kind, siteID: siteID}
	var output io.ReadCloser
	{
		var err error
		cmd.Stdout = nil
		output, err = cmd.StdoutPipe()
		if err != nil {
			return err
		}
	}
	if kind == "quick" || kind == "viewer" || kind == "host" || kind == "terminal-window" {
		var err error
		p.stdin, err = cmd.StdinPipe()
		if err != nil {
			output.Close()
			return err
		}
	}
	if err := cmd.Start(); err != nil {
		if output != nil {
			output.Close()
		}
		if p.stdin != nil {
			p.stdin.Close()
		}
		return err
	}
	s.children[key] = p
	s.notice = ""
	go func() {
		if output != nil {
			scanner := bufio.NewScanner(output)
			scanner.Buffer(make([]byte, 4096), 16<<20)
			for scanner.Scan() {
				line := scanner.Text()
				if payload, ok := strings.CutPrefix(line, agentremote.Prefix); ok {
					var response agentremote.Response
					if json.Unmarshal([]byte(payload), &response) == nil {
						s.mu.Lock()
						if ch := p.agentPending[response.ID]; ch != nil {
							select {
							case ch <- response:
							default:
							}
						}
						s.mu.Unlock()
					}
					continue
				}
				if payload, ok := strings.CutPrefix(line, "YOURDESK_UI_EVENT "); ok {
					var event struct {
						Session   uint64               `json:"session"`
						Local     bool                 `json:"local"`
						Sample    *diagnostics.Sample  `json:"sample"`
						Proof     string               `json:"proof"`
						Event     string               `json:"event"`
						Message   string               `json:"message"`
						Transfers []clipboard.Progress `json:"transfers"`
					}
					if json.Unmarshal([]byte(payload), &event) == nil {
						s.mu.Lock()
						switch event.Event {
						case "host-conflict":
							if p.kind == "host" {
								s.notice = "此裝置已有另一個 Host 連線，請結束原程式後再試。"
								s.hostConflict = nil
								if event.Local {
									s.hostConflict = hostguard.FindOwner(s.info["room"])
								}
							}
						case "host-connected":
							if p.kind == "host" && event.Session >= p.incomingGeneration {
								p.incomingGeneration = event.Session
								p.incomingConnected = true
							}
						case "host-disconnected":
							if p.kind == "host" && event.Session == p.incomingGeneration {
								p.incomingConnected = false
							}
						case "host-ready":
							if p.kind == "host" {
								s.hostConflict = nil
							}
							if p.kind == "host" && s.notice == "此裝置已有另一個 Host 連線，請結束原程式後再試。" {
								s.notice = ""
							}
						case "host-password-proof":
							if p.kind == "host" {
								authlog.Event("host-password-match", map[string]any{"matchesUI": p.passwordProof != "" && subtle.ConstantTimeCompare([]byte(p.passwordProof), []byte(event.Proof)) == 1, "childPid": p.cmd.Process.Pid})
								p.passwordProof = ""
							}
						case "diagnostic-sample":
							if d := p.diagnostic; d != nil && event.Sample != nil && event.Sample.StartedAt >= d.StartedAt && time.Now().UnixMilli()-d.StartedAt <= 30000 && len(d.Samples) < 6 {
								d.Samples = append(d.Samples, *event.Sample)
							}
						case "quit-application":
							if (p.kind == "viewer" || p.kind == "quick") && s.closeApplication != nil {
								s.closeApplication()
							}
						case "clipboard-progress":
							// 傳輸進度由各自的 遠端顯示 顯示，不喚起管理主畫面。
						case "error":
							p.failureMessage = event.Message
							s.notice = event.Message
						case "disconnected":
							p.stage = "disconnected"
						case "authenticated":
							if err := s.remember(p); err != nil {
								s.notice = "無法保存連線密碼"
							}

							p.authRequired = false
							p.stage = "connecting"
						case "password-required":
							p.authRequired = true
							p.authMessage = event.Message
							p.stage = "password"
						case "frame":
							// 首張畫面準備好才隱藏主介面，密碼驗證成功不代表視窗已開啟。
							if p.stage != "connected" && !p.mcpOwned && !p.diagnosticConnection && !p.terminalConnection {
								select {
								case s.connected <- struct{}{}:
								default:
								}
							}
							p.authRequired = false
							p.stage = "connected"
						}
						s.mu.Unlock()
					}
				} else {
					fmt.Fprintln(os.Stdout, line)
				}
			}
			output.Close()
		}
		err := cmd.Wait()
		if p.stdin != nil {
			p.stdin.Close()
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.children[key] == p {
			delete(s.children, key)
		}
		close(p.done)
		if p.terminalConnection && s.preferences.CloseWindowOnDisconnect {
			for _, window := range s.children {
				if window.kind == "terminal-window" && window.terminalOwner == p {
					_ = window.cmd.Process.Kill()
				}
			}
		}
		if p.kind == "terminal-window" && p.terminalOwner != nil {
			if remote := s.children["viewer:"+p.terminalOwner.siteID]; remote != nil && remote.terminalConnection && remote == p.terminalOwner {
				_ = remote.cmd.Process.Kill()
			}
		}
		// 遠端結束時保留視窗供閱讀最後輸出，視窗的輪詢會顯示斷線狀態。
		if !p.mcpOwned && !p.diagnosticConnection && !p.terminalConnection && (p.kind == "viewer" || p.kind == "quick") {
			select {
			case s.viewerClosed <- struct{}{}:
			default:
			}
		}
		if err != nil && p.failureMessage == "" && !p.diagnosticConnection && !p.terminalConnection && p.kind != "terminal-window" {
			s.notice = "程序已結束；若非手動停止，請查看啟動終端的錯誤訊息。"
		}
	}()
	return nil
}

func sendProcessSecret(p *process, secret string) error {
	if p == nil || p.stdin == nil {
		return errors.New("子程序的密碼通道尚未就緒")
	}
	payload := map[string]string{"secret": secret}
	if p.kind == "host" && authlog.Enabled == "1" {
		nonce := make([]byte, 32)
		if _, err := rand.Read(nonce); err != nil {
			return errors.New("無法建立程序驗證資料")
		}
		key, err := security.DecodeSecret(secret)
		if err != nil {
			return err
		}
		payload["challenge"] = hex.EncodeToString(nonce)
		p.passwordProof = security.Sign(key, []byte(payload["challenge"]))
	}
	if err := json.NewEncoder(p.stdin).Encode(payload); err != nil {
		return errors.New("無法透過私有管道傳送連線密碼")
	}
	authlog.Event("password-sent", map[string]any{"kind": p.kind, "childPid": p.cmd.Process.Pid})
	return nil
}

// APP 開啟期間持續維持被控端程序，重試間隔避免故障時密集啟動。
func (s *server) keepHostRunning(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		managed := s.preloginBusy || prelogin.Status().Enabled
		s.mu.Unlock()
		if managed {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}
		if !s.waitHostPreflight(ctx) {
			return
		}
		s.mu.Lock()
		if s.preloginBusy || prelogin.Status().Enabled {
			s.mu.Unlock()
			continue
		}
		args := append(append([]string{}, s.options.HostArgs...), "-codec", s.preferences.Codec, "-secret-stdin", "-parent-stdin")
		if s.preferences.TailcatEnabled {
			args = append(args, "-transport", "tailcat")
		}
		if s.preferences.DirectListen {
			address := s.options.DirectListen
			if address == "" {
				address = ":" + signaling.DirectPort
			}
			args = append(args, "-direct-listen", address)
		}
		err := s.start("host", "", s.executable, args)
		child := s.children["host"]
		if err == nil {
			// 使用介面已載入的同一份密碼，子程序不再自行讀取另一份設定。
			if err = sendProcessSecret(child, s.options.Secret); err != nil {
				_ = child.cmd.Process.Kill()
			}
		}
		if err != nil {
			s.notice = "本機 Client 啟動失敗，5 秒後重試；請查看啟動終端。"
			fmt.Fprintln(os.Stderr, err)
		}
		s.mu.Unlock()
		if child != nil {
			ticker := time.NewTicker(time.Second)
			waiting := true
			for waiting {
				select {
				case <-ctx.Done():
					ticker.Stop()
					return
				case <-child.done:
					waiting = false
				case <-ticker.C:
					if prelogin.Status().Enabled {
						child.stdin.Close()
					}
				}
			}
			ticker.Stop()
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *server) viewerBinary() string {
	name := "yourdesk-remote"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(s.executable), name)
}

func (p Preferences) validate() error {
	if p.MCPWhitelistEnabled && len(p.MCPWhitelist) == 0 {
		return errors.New("請至少加入一個允許的 IP 位址")
	}
	if len(p.MCPWhitelist) > 128 {
		return errors.New("白名單最多可加入 128 個 IP 位址")
	}
	for _, address := range p.MCPWhitelist {
		if net.ParseIP(address) == nil {
			return errors.New("白名單含無效的 IP 位址")
		}
	}

	if p.SourceFPSLimit < 5 || p.SourceFPSLimit > 60 {
		return errors.New("FPS 上限必須為 5 至 60")
	}
	if p.BitrateLimitMbps < 1 || p.BitrateLimitMbps > 64 {
		return errors.New("碼率上限必須為 1 至 64 Mbps")
	}
	if p.KeyframeInterval < 0 || p.KeyframeInterval > 300 {
		return errors.New("GOP 必須為 1 至 300 張")
	}
	switch p.EnhancementStrategy {
	case "", "quality", "smooth", "traffic":
	default:
		return errors.New("不支援的增強策略")
	}
	if p.EnhancementBitrateMbps != 0 && (p.EnhancementBitrateMbps < 1 || p.EnhancementBitrateMbps > 100) {
		return errors.New("基準碼率必須為 1 至 100 Mbps")
	}

	switch p.InterpolationMethod {
	case "", "apple", "rife":
	default:
		return errors.New("不支援的補幀方式")
	}
	switch p.CoreMLModel {
	case "", "quicksrnet-small", "sesr-m5":
	default:
		return errors.New("不支援的 Core ML 模型")
	}
	switch p.SuperResolution {
	case "", "fsr1", "coreml":
	default:
		return errors.New("不支援的超解析度方式")
	}
	// 設定驗證只檢查格式；硬體可用性由背景探測及串流協商決定。
	// 暫時不可用不可阻止讀取已保存的設定或啟動主畫面。
	switch p.Codec {
	case "", "auto", "software-jpeg", "hardware-h264", "hardware-hevc", "hardware-jpeg":
	default:
		return errors.New("不支援此影像傳輸方式")
	}

	switch p.Language {
	case "auto", "zh-Hant", "en", "ja", "ko":
	default:
		return errors.New("不支援的介面語言")
	}
	switch p.Theme {
	case "default", "light", "dark":
	default:
		return errors.New("不支援的介面風格")
	}
	return nil
}

// 配對位址只留在後端，不提供給介面。
func publicLibrary(library Library) Library {
	result := library
	result.Sites = append([]Site(nil), library.Sites...)
	for i := range result.Sites {
		result.Sites[i].Signal = ""
	}
	return result
}

func (s *server) passwordPrompt() any {
	for key, p := range s.children {
		if p.authRequired {
			return map[string]any{"key": key, "room": p.room, "message": p.authMessage, "remember": p.remember}
		}
	}
	return nil
}
