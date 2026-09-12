package prelogin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unsafe"

	winio "github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var openInputDesktop = user32.NewProc("OpenInputDesktop")
var closeDesktop = user32.NewProc("CloseDesktop")
var getUserObjectInformation = user32.NewProc("GetUserObjectInformationW")

// 代理只查詢前景桌面名稱；Host 直接在該桌面建立，避免在已有視窗的執行緒切桌面。
func inputDesktopName() (string, error) {
	h, _, err := openInputDesktop.Call(0, 0, 0x0001) // DESKTOP_READOBJECTS
	if h == 0 {
		return "", err
	}
	defer closeDesktop.Call(h)
	var name [256]uint16
	var needed uint32
	ok, _, err := getUserObjectInformation.Call(h, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Sizeof(name)), uintptr(unsafe.Pointer(&needed)))
	if ok == 0 {
		return "", err
	}
	value := windows.UTF16ToString(name[:])
	if value == "" || strings.ContainsAny(value, `\/`+"\x00") {
		return "", errors.New("無效的桌面名稱")
	}
	return value, nil
}

func runAgent(parent context.Context) error {
	if err := requireServiceProcess(); err != nil {
		return err
	}
	if len(os.Args) != 4 || !strings.HasPrefix(os.Args[3], `\\.\pipe\YourDeskPrelogin-`) {
		return errors.New("代理缺少服務控制管線")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	connect, done := context.WithTimeout(ctx, 10*time.Second)
	conn, err := winio.DialPipeContext(connect, os.Args[3])
	done()
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 64<<10)
	var config Config
	if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &config) != nil {
		return errors.New("無法讀取服務設定")
	}
	if err = config.validate(); err != nil {
		return err
	}
	conn.SetReadDeadline(time.Time{})
	disconnect := make(chan struct{}, 1)
	go func() {
		defer cancel()
		for scanner.Scan() {
			var command struct {
				Disconnect bool `json:"disconnect"`
			}
			if json.Unmarshal(scanner.Bytes(), &command) == nil && command.Disconnect {
				select {
				case disconnect <- struct{}{}:
				default:
				}
			}
		}
	}()
	var session uint32
	if err = windows.ProcessIdToSessionId(uint32(os.Getpid()), &session); err != nil {
		return err
	}
	args := []string{"-signal", config.Signal, "-room", config.Room, "-codec", config.Codec, "-secret-stdin", "-parent-stdin", "-prelogin-host"}
	if config.Direct {
		args = append(args, "-direct-listen", ":47823")
	}
	if config.Tailcat {
		args = append(args, "-transport", "tailcat")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var host *serviceChild
	var hostDone <-chan struct{}
	var events chan serviceEvent
	var desktop string
	var generation uint64
	var eventCancel context.CancelFunc
	stopHost := func() {
		if host != nil {
			eventCancel()
			host.stop()
			host = nil
		}
		hostDone, events = nil, nil
		generation = 0
		conn.SetWriteDeadline(time.Now().Add(time.Second))
		if _, err := fmt.Fprintln(conn, `YOURDESK_UI_EVENT {"event":"host-disconnected"}`); err != nil {
			cancel()
		}
	}
	defer stopHost()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-hostDone:
			stopHost()
		case <-disconnect:
			if host != nil {
				if host.send(map[string]bool{"disconnect": true}) != nil {
					stopHost()
				}
			}
		case event := <-events:
			// 舊連線的延遲斷線事件不能覆蓋較新的連線狀態。
			if event.Event == "host-connected" && event.Session >= generation {
				generation = event.Session
			} else if event.Event != "host-disconnected" || event.Session != generation {
				continue
			}
			payload, _ := json.Marshal(event)
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			if _, err := fmt.Fprintln(conn, "YOURDESK_UI_EVENT "+string(payload)); err != nil {
				return err
			}
		case <-ticker.C:
			if windows.WTSGetActiveConsoleSessionId() != session {
				return nil
			}
			name, err := inputDesktopName()
			if err != nil {
				if host != nil {
					stopHost()
				}
				continue
			}
			if host != nil && !strings.EqualFold(name, desktop) {
				stopHost()
			}
			if host != nil {
				continue
			}
			host, err = startServiceChild(session, name, args)
			if err != nil {
				slog.Warn("桌面代理等待重試", "error", err)
				continue
			}
			desktop, hostDone = name, host.done
			var eventContext context.Context
			eventContext, eventCancel = context.WithCancel(ctx)
			events = make(chan serviceEvent, 16)
			go watchServiceOutput(eventContext, host, events)
			if err = host.send(map[string]string{"secret": config.Secret}); err != nil {
				stopHost()
			}
		}
	}
}
