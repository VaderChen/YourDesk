package prelogin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	winio "github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

const controlPipe = `\\.\pipe\YourDeskPrelogin-control`

type windowsService struct{ ctx context.Context }

func runDaemon(ctx context.Context) error {
	if err := requireServiceProcess(); err != nil {
		return err
	}
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		return errors.New("請透過 Windows 服務管理員啟動")
	}
	return svc.Run(serviceName, &windowsService{ctx: ctx})
}

func (s *windowsService) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending, WaitHint: 10000}
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	root, _, err := servicePaths()
	if err != nil {
		return false, 1
	}
	data, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil {
		return false, 1
	}
	var config Config
	if json.Unmarshal(data, &config) != nil || config.validate() != nil {
		return false, 1
	}
	// 本機互動使用者僅能查詢狀態或要求斷線；不提供密碼、程序啟動或設定寫入。
	listener, err := winio.ListenPipe(controlPipe, &winio.PipeConfig{SecurityDescriptor: "D:P(D;;GA;;;NU)(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;IU)"})
	if err != nil {
		slog.Error("服務控制管線無法啟動", "error", err)
		return false, 1
	}
	defer listener.Close()
	var mu sync.Mutex
	connected := false
	disconnect := make(chan struct{}, 1)
	go serveControl(ctx, listener, func(operation string) bool {
		mu.Lock()
		defer mu.Unlock()
		if operation == "disconnect" {
			select {
			case disconnect <- struct{}{}:
			default:
			}
		}
		return connected
	})
	setConnected := func(value bool) { mu.Lock(); connected = value; mu.Unlock() }
	status := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown | svc.AcceptSessionChange}
	statuses <- status
	var agent *serviceChild
	var agentDone <-chan struct{}
	var events chan serviceEvent
	var eventCancel context.CancelFunc
	var session uint32
	stopAgent := func() {
		if agent != nil {
			eventCancel()
			agent.stop()
			agent = nil
		}
		agentDone, events = nil, nil
		setConnected(false)
	}
	defer stopAgent()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			statuses <- svc.Status{State: svc.StopPending, WaitHint: 10000}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending, WaitHint: 10000}
				return false, 0
			case svc.Interrogate:
				statuses <- status
			}
		case <-agentDone:
			stopAgent()
		case event := <-events:
			setConnected(event.Event == "host-connected")
		case <-disconnect:
			if agent != nil {
				if agent.send(map[string]bool{"disconnect": true}) != nil {
					stopAgent()
				}
			}
		case <-ticker.C:
			current := windows.WTSGetActiveConsoleSessionId()
			if agent != nil && session != current {
				stopAgent()
			}
			if agent != nil || current == 0 || current == 0xffffffff {
				continue
			}
			agent, err = startServiceChild(current, "default", []string{"--prelogin", "agent"})
			if err != nil {
				slog.Warn("等待主控台代理", "error", err)
				continue
			}
			session, agentDone = current, agent.done
			var eventContext context.Context
			eventContext, eventCancel = context.WithCancel(ctx)
			events = make(chan serviceEvent, 16)
			go watchServiceOutput(eventContext, agent, events)
			if err = agent.send(config); err != nil {
				stopAgent()
			}
		}
	}
}

func serveControl(ctx context.Context, listener net.Listener, operation func(string) bool) {
	// 限制未完成請求數量與期限，避免本機程式累積無上限的工作。
	slots := make(chan struct{}, 8)
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		select {
		case slots <- struct{}{}:
		default:
			conn.Close()
			continue
		}
		go func() {
			defer conn.Close()
			defer func() { <-slots }()
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			var request struct {
				Operation string `json:"operation"`
			}
			if json.NewDecoder(io.LimitReader(conn, 1024)).Decode(&request) != nil || ctx.Err() != nil {
				return
			}
			if request.Operation != "status" && request.Operation != "disconnect" {
				return
			}
			_ = json.NewEncoder(conn).Encode(operation(request.Operation))
		}()
	}
}

func Incoming(ctx context.Context, disconnect bool) (bool, error) {
	operation := "status"
	if disconnect {
		operation = "disconnect"
	}
	return brokerRequest(ctx, operation)
}

func brokerRequest(ctx context.Context, operation string) (bool, error) {
	if operation != "status" && operation != "disconnect" {
		return false, errors.New("未知服務操作")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, controlPipe)
	if err != nil {
		return false, errors.New("登入前服務尚未就緒，請稍後重試。")
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	if err = json.NewEncoder(conn).Encode(map[string]string{"operation": operation}); err != nil {
		return false, errors.New(serviceFailed)
	}
	var connected bool
	if err = json.NewDecoder(io.LimitReader(conn, 1024)).Decode(&connected); err != nil {
		return false, errors.New(serviceFailed)
	}
	return connected, nil
}
