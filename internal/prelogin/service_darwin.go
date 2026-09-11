//go:build darwin && cgo

package prelogin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const root = "/Library/Application Support/YourDeskPrelogin"
const app = root + "/YourDesk.app"
const executable = app + "/Contents/MacOS/yourdesk-client"
const configFile = root + "/config.json"
const socket = root + "/broker.sock"
const daemonLabel = "com.yourdesk.prelogin.broker"
const agentLabel = "com.yourdesk.prelogin.agent"
const daemonFile = "/Library/LaunchDaemons/" + daemonLabel + ".plist"
const agentFile = "/Library/LaunchAgents/" + agentLabel + ".plist"

func consoleUID() (uint32, error) {
	var s unix.Stat_t
	err := unix.Stat("/dev/console", &s)
	return s.Uid, err
}
func secureRoot(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	raw, ok := st.Sys().(*syscall.Stat_t)
	if !ok || raw.Uid != 0 || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("服務路徑權限不安全：%s", path)
	}
	return nil
}
func installed() bool {
	for _, p := range []string{root, configFile, executable, daemonFile, agentFile} {
		if secureRoot(p) != nil {
			return false
		}
	}
	return true
}
func sourceApp() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	for p = filepath.Dir(p); p != filepath.Dir(p); p = filepath.Dir(p) {
		if filepath.Ext(p) == ".app" {
			if filepath.Base(p) != "YourDesk.app" {
				break
			}
			return p, nil
		}
	}
	return "", errors.New("請從已簽署的 YourDesk.app 啟用未登入連線。")
}
func verifyApp(ctx context.Context, p string) error {
	// 僅接受 Apple 信任鏈的 Developer ID 簽章；不提升臨時開發執行檔。
	return exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", "-R", "anchor apple generic", p).Run()
}

var versionOnce sync.Once
var supportedVersion bool

func Status() State {
	s := State{Enabled: secureRoot(root) == nil}
	versionOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		data, err := exec.CommandContext(ctx, "/usr/bin/sw_vers", "-productVersion").Output()
		if err != nil {
			return
		}
		parts := strings.Split(strings.TrimSpace(string(data)), ".")
		major, _ := strconv.Atoi(parts[0])
		minor := 0
		if len(parts) > 1 {
			minor, _ = strconv.Atoi(parts[1])
		}
		supportedVersion = major > 14 || (major == 14 && minor >= 4)
	})
	if !supportedVersion {
		s.Message = "未登入連線需要 macOS 14.4 以上。"
		return s
	}
	if s.Enabled {
		var public struct {
			Room string `json:"room"`
		}
		if data, err := os.ReadFile(root + "/public.json"); err == nil {
			_ = json.Unmarshal(data, &public)
			s.Room = public.Room
		}
	}
	if !s.Enabled {
		if _, err := sourceApp(); err != nil {
			s.Message = err.Error()
			return s
		}
	}
	s.Supported = true
	s.Message = "實驗性功能：需管理員授權及螢幕錄製、輔助使用權限；不支援 FileVault 開機前畫面。"
	return s
}
func Configure(ctx context.Context, enabled bool, c Config) error {
	if !Status().Supported {
		return errors.New(Status().Message)
	}
	if !enabled {
		return elevate(ctx, "remove", "")
	}
	if err := c.validate(); err != nil {
		return err
	}
	source, err := sourceApp()
	if err != nil {
		return err
	}
	if err = verifyApp(ctx, source); err != nil {
		return errors.New("APP 簽章驗證失敗，請使用正式簽署版本。")
	}
	stage, err := os.MkdirTemp("", "yourdesk-prelogin-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	data, _ := json.Marshal(c)
	path := filepath.Join(stage, "config.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return elevate(ctx, "install", path)
}
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func elevate(ctx context.Context, mode, path string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	command := shellQuote(self) + " --prelogin " + mode
	if path != "" {
		command += " " + shellQuote(path)
	}
	// AppleScript 字串與 shell 參數分別編碼，不插入密碼。
	quoted := strconv.Quote(command)
	err = exec.CommandContext(ctx, "/usr/bin/osascript", "-e", "do shell script "+quoted+" with administrator privileges").Run()
	if err != nil {
		return fmt.Errorf("服務操作未完成或管理員授權已取消：%w", err)
	}
	return nil
}
func plist(label, mode string) []byte {
	sessions := ""
	if mode == "agent" {
		sessions = "<key>LimitLoadToSessionType</key><array><string>Aqua</string><string>LoginWindow</string></array>"
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>` + label + `</string><key>ProgramArguments</key><array><string>` + executable + `</string><string>--prelogin</string><string>` + mode + `</string></array><key>RunAtLoad</key><true/><key>KeepAlive</key><dict><key>PathState</key><dict><key>/Library/Application Support/YourDeskPrelogin/config.json</key><true/></dict></dict><key>ThrottleInterval</key><integer>5</integer><key>ExitTimeOut</key><integer>30</integer><key>ProcessType</key><string>Interactive</string>` + sessions + `</dict></plist>`)
}
func launch(args ...string) error { return exec.Command("/bin/launchctl", args...).Run() }
func install(path string) error {
	if os.Geteuid() != 0 {
		return errors.New("安裝服務需要管理員權限")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c Config
	if err = json.Unmarshal(data, &c); err != nil {
		return err
	}
	if err = c.validate(); err != nil {
		return err
	}
	if _, err = os.Lstat(root); err == nil {
		return errors.New("服務目錄已存在；請先停用或由管理員檢查，不覆蓋既有服務")
	}
	for _, p := range []string{"/Library", "/Library/Application Support", "/Library/LaunchDaemons", "/Library/LaunchAgents"} {
		if err = secureRoot(p); err != nil {
			return err
		}
	}
	source, err := sourceApp()
	if err != nil {
		return err
	}
	if err = verifyApp(context.Background(), source); err != nil {
		return err
	}
	for _, p := range []string{daemonFile, agentFile} {
		if _, e := os.Lstat(p); !os.IsNotExist(e) {
			return errors.New("啟動服務檔案已存在，請由管理員檢查")
		}
	}
	if err = os.Mkdir(root, 0755); err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			_ = remove()
		}
	}()
	if err = exec.Command("/usr/bin/ditto", source, app).Run(); err != nil {
		return err
	}
	if err = exec.Command("/usr/sbin/chown", "-R", "root:wheel", root).Run(); err != nil {
		return err
	}
	if err = exec.Command("/bin/chmod", "-R", "go-w", root).Run(); err != nil {
		return err
	}
	if err = verifyApp(context.Background(), app); err != nil {
		return err
	}
	public, _ := json.Marshal(map[string]string{"room": c.Room})
	if err = os.WriteFile(root+"/public.json", public, 0644); err != nil {
		return err
	}
	if err = os.WriteFile(root+"/host.lock", nil, 0644); err != nil {
		return err
	}
	if err = os.WriteFile(configFile, data, 0600); err != nil {
		return err
	}
	if err = os.WriteFile(daemonFile, plist(daemonLabel, "daemon"), 0644); err != nil {
		return err
	}
	if err = os.WriteFile(agentFile, plist(agentLabel, "agent"), 0644); err != nil {
		return err
	}
	if err = launch("bootstrap", "system", daemonFile); err != nil {
		return err
	}
	uid, err := consoleUID()
	if err != nil {
		return err
	}
	domain := ""
	if uid != 0 {
		domain = fmt.Sprintf("gui/%d", uid)
	}
	if domain != "" {
		if err = launch("bootstrap", domain, agentFile); err != nil {
			return fmt.Errorf("無法載入桌面代理：%w", err)
		}
	}
	success = true
	return nil
}
func remove() error {
	if os.Geteuid() != 0 {
		return errors.New("停用服務需要管理員權限")
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	}
	if err := secureRoot(root); err != nil {
		return err
	}
	// broker 停止時會撤銷所有租約；其他登入工作階段不再持有有效 Host。
	if launch("print", "system/"+daemonLabel) == nil {
		if err := launch("bootout", "system/"+daemonLabel); err != nil {
			return fmt.Errorf("無法停止系統服務：%w", err)
		}
	}
	if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = launch("unload", "-S", "LoginWindow", agentFile)
	if uid, err := consoleUID(); err == nil && uid != 0 {
		_ = launch("bootout", fmt.Sprintf("gui/%d/%s", uid, agentLabel))
	}
	// 父管線關閉後 Host 最遲五秒結束，先等它釋放註冊。
	time.Sleep(6 * time.Second)
	for _, p := range []string{agentFile, daemonFile} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	f, err := os.Open(root + "/host.lock")
	if err == nil {
		defer f.Close()
		if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
			return errors.New("Host 尚未結束，已保留服務目錄避免重複啟動；請由管理員檢查。")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.RemoveAll(root)
}
func peerIdentity(c *net.UnixConn) (uint32, int, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, 0, err
	}
	var uid uint32
	var pid int
	var e error
	err = raw.Control(func(fd uintptr) {
		var cred *unix.Xucred
		cred, e = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if e == nil {
			uid = cred.Uid
			pid, e = unix.GetsockoptInt(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERPID)
		}
	})
	if err != nil {
		return 0, 0, err
	}
	return uid, pid, e
}
func runDaemon(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()

	if os.Geteuid() != 0 || !installed() {
		return errors.New("服務未正確安裝")
	}
	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}
	var c Config
	if err = json.Unmarshal(data, &c); err != nil {
		return err
	}
	if err = c.validate(); err != nil {
		return err
	}
	_ = os.Remove(socket)
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		return err
	}
	defer l.Close()
	if err = os.Chmod(socket, 0666); err != nil {
		return err
	}
	go func() { <-ctx.Done(); l.Close() }()
	// 等待其他已登入 UI 回收原本的 Host。
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(6 * time.Second):
	}
	var lease sync.Mutex
	var connectionMu sync.Mutex
	var target *net.UnixConn
	var connected bool
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer conn.Close()
			uid, pid, err := peerIdentity(conn)
			if err != nil || processPath(pid) != executable {
				return
			}
			currentUID, e := consoleUID()
			if e != nil || uid != currentUID {
				return
			}
			decoder := json.NewDecoder(io.LimitReader(conn, 1<<20))
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			var request struct {
				Operation string `json:"operation"`
			}
			if decoder.Decode(&request) != nil {
				return
			}
			conn.SetReadDeadline(time.Time{})
			if request.Operation == "status" || request.Operation == "disconnect" {
				connectionMu.Lock()
				active := connected
				if request.Operation == "disconnect" && connected && target != nil {
					target.SetWriteDeadline(time.Now().Add(time.Second))
					if json.NewEncoder(target).Encode(map[string]bool{"disconnect": true}) != nil {
						connectionMu.Unlock()
						return
					}
				}
				connectionMu.Unlock()
				conn.SetWriteDeadline(time.Now().Add(time.Second))
				_ = json.NewEncoder(conn).Encode(active)
				return
			}
			if request.Operation != "agent" {
				return
			}
			if !lease.TryLock() {
				return
			}
			defer lease.Unlock()
			current, err := consoleUID()
			if err != nil || uid != current {
				return
			}
			agentStarted := processStarted(pid)
			if agentStarted == 0 {
				return
			}
			defer func() {
				conn.Close()
				if processPath(pid) == executable && processStarted(pid) == agentStarted {
					_ = unix.Kill(pid, unix.SIGKILL)
				}
				// 即使代理在回報 PID 前失敗，父管線仍會讓 Host 最遲五秒退出。
				time.Sleep(6 * time.Second)
			}()
			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if json.NewEncoder(conn).Encode(c) != nil {
				return
			}
			// 記錄真正的直屬 Host，工作階段切換時由 broker 保證回收。
			var started struct {
				PID int `json:"pid"`
			}
			conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			if decoder.Decode(&started) != nil {
				return
			}
			conn.SetReadDeadline(time.Time{})
			hostPID := started.PID
			if hostPID <= 0 || processPath(hostPID) != executable || processParent(hostPID) != pid {
				return
			}
			hostStarted := processStarted(hostPID)
			defer func() {
				if hostStarted != 0 && processPath(hostPID) == executable && processStarted(hostPID) == hostStarted {
					_ = unix.Kill(hostPID, unix.SIGKILL)
				}
			}()
			// 只授權目前圖形工作階段。socket EOF 同時是回收 Host 的指令。
			connectionMu.Lock()
			target = conn
			connected = false
			connectionMu.Unlock()
			defer func() {
				connectionMu.Lock()
				if target == conn {
					target = nil
					connected = false
				}
				connectionMu.Unlock()
			}()
			sessionGeneration := uint64(0)
			ended := make(chan struct{})
			go func() {
				defer close(ended)
				for {
					var event struct {
						Event   string `json:"event"`
						Session uint64 `json:"session"`
					}
					if decoder.Decode(&event) != nil {
						return
					}
					connectionMu.Lock()
					if target == conn {
						if event.Event == "host-connected" && event.Session >= sessionGeneration {
							sessionGeneration = event.Session
							connected = true
						}
						if event.Event == "host-disconnected" && event.Session == sessionGeneration {
							connected = false
						}
					}
					connectionMu.Unlock()
				}
			}()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			alive := true
			for alive {
				select {
				case <-ctx.Done():
					alive = false
				case <-ended:
					alive = false
				case <-ticker.C:
					now, e := consoleUID()
					if e != nil || now != uid {
						alive = false
					}
				}
			}
			conn.Close()
			if processPath(hostPID) == executable && processParent(hostPID) == pid {
				_ = unix.Kill(hostPID, unix.SIGKILL)
			}

		}()
	}
}
func runAgent(ctx context.Context) error {
	self, err := os.Executable()
	if err != nil || self != executable {
		return errors.New("桌面代理只能從受保護的安裝位置啟動")
	}
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		return err
	}
	defer conn.Close()
	uid, pid, err := peerIdentity(conn)
	if err != nil || uid != 0 || processPath(pid) != executable {
		return errors.New("無法驗證系統服務")
	}
	if err = json.NewEncoder(conn).Encode(map[string]string{"operation": "agent"}); err != nil {
		return err
	}
	var c Config
	decoder := json.NewDecoder(conn)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err = decoder.Decode(&c); err != nil {
		return err
	}
	conn.SetReadDeadline(time.Time{})
	if err = c.validate(); err != nil {
		return err
	}
	args := []string{"-signal", c.Signal, "-room", c.Room, "-codec", c.Codec, "-secret-stdin", "-parent-stdin", "-prelogin-host"}
	if c.Direct {
		args = append(args, "-direct-listen", ":47823")
	}
	worker, stop := context.WithCancel(ctx)
	defer stop()
	cmd := exec.CommandContext(worker, executable, args...)
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer pipe.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	defer output.Close()
	// 只將固定的連線狀態回報 broker，不轉送密碼或其他輸出。
	if err = cmd.Start(); err != nil {
		return err
	}
	if err = json.NewEncoder(conn).Encode(map[string]int{"pid": cmd.Process.Pid}); err != nil {
		stop()
	}
	if err = json.NewEncoder(pipe).Encode(map[string]string{"secret": c.Secret}); err != nil {
		stop()
	}
	go func() {
		defer stop()
		for {
			var request struct {
				Disconnect bool `json:"disconnect"`
			}
			if decoder.Decode(&request) != nil {
				return
			}
			if request.Disconnect {
				if json.NewEncoder(pipe).Encode(map[string]bool{"disconnect": true}) != nil {
					return
				}
			}
		}
	}()
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		payload, ok := strings.CutPrefix(scanner.Text(), "YOURDESK_UI_EVENT ")
		if !ok {
			continue
		}
		var event struct {
			Event   string `json:"event"`
			Session uint64 `json:"session"`
		}
		if json.Unmarshal([]byte(payload), &event) != nil {
			continue
		}
		if event.Event == "host-connected" || event.Event == "host-disconnected" {
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			if json.NewEncoder(conn).Encode(event) != nil {
				stop()
				break
			}
		}
	}
	return cmd.Wait()
}

// 所有工作階段的 Host 共用不可由一般使用者替換的鎖檔。
// 即使 broker 異常重啟，鎖仍由真正的 Host 持有到程序結束。
func AcquireHost(ctx context.Context) (func(), error) {
	self, err := os.Executable()
	if err != nil || self != executable {
		return nil, errors.New("無效的服務 Host 路徑")
	}
	if err = secureRoot(root + "/host.lock"); err != nil {
		return nil, err
	}
	f, err := os.Open(root + "/host.lock")
	if err != nil {
		return nil, err
	}
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() { f.Close() }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// UI 透過安裝位置的固定 helper 查詢／中斷，不取得服務密碼。
func Incoming(ctx context.Context, disconnect bool) (bool, error) {
	operation := "status"
	if disconnect {
		operation = "disconnect"
	}
	data, err := exec.CommandContext(ctx, executable, "--prelogin", operation).Output()
	if err != nil {
		return false, err
	}
	var active bool
	err = json.Unmarshal(data, &active)
	return active, err
}
func brokerRequest(ctx context.Context, operation string) (bool, error) {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		return false, err
	}
	defer conn.Close()
	uid, pid, err := peerIdentity(conn)
	if err != nil || uid != 0 || processPath(pid) != executable {
		return false, errors.New("無法驗證系統服務")
	}
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	if err = json.NewEncoder(conn).Encode(map[string]string{"operation": operation}); err != nil {
		return false, err
	}
	var active bool
	err = json.NewDecoder(conn).Decode(&active)
	return active, err
}
