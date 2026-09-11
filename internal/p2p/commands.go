package p2p

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const CommandVersion = 1

var ErrCommandUnsupported = errors.New("對端未提供此 P2P 指令")

type CommandCapabilities struct {
	Version int      `json:"version"`
	Methods []string `json:"methods"`
}
type CommandRequest struct {
	Params  json.RawMessage `json:"params,omitempty"`
	Version int             `json:"version"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Expires int64           `json:"expires"`
}
type CommandResponse struct {
	Version int             `json:"version"`
	ID      uint64          `json:"id"`
	Code    string          `json:"code,omitempty"`
	Error   string          `json:"error,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}
type CommandHandler func(context.Context) (any, error)
type commandState struct {
	mu             sync.Mutex
	handlers       map[string]func(context.Context, json.RawMessage) (any, error)
	remote         CommandCapabilities
	pending        map[uint64]chan CommandResponse
	received       map[uint64]*CommandResponse
	sequence, high uint64
	active         int
}

func (p *Peer) commandsInit() {
	p.commandsOnce.Do(func() {
		p.commands.handlers = map[string]func(context.Context, json.RawMessage) (any, error){}
		p.commands.pending = map[uint64]chan CommandResponse{}
		p.commands.received = map[uint64]*CommandResponse{}
	})
}
func (p *Peer) localCommands() CommandCapabilities {
	p.commandsInit()
	p.commands.mu.Lock()
	defer p.commands.mu.Unlock()
	methods := []string{"capabilities.get", "ping", "session.status", "session.disconnect"}
	for name := range p.commands.handlers {
		methods = append(methods, name)
	}
	sort.Strings(methods)
	return CommandCapabilities{CommandVersion, methods}
}
func (p *Peer) announceCommands() {
	_ = p.SendControl(Control{Type: "command-capabilities", CommandCapabilities: ptrCapabilities(p.localCommands())})
}
func ptrCapabilities(c CommandCapabilities) *CommandCapabilities { return &c }

// RegisterCommand 只註冊此端實際具備的功能；不以名稱推定支援。
func (p *Peer) RegisterCommand(name string, handler CommandHandler) error {
	if handler == nil {
		return errors.New("無效的 P2P 指令")
	}
	return p.RegisterCommandParams(name, func(ctx context.Context, params json.RawMessage) (any, error) {
		if len(params) != 0 && string(params) != "null" && string(params) != "{}" {
			return nil, errors.New("此指令不接受參數")
		}
		return handler(ctx)
	})
}
func (p *Peer) RegisterCommandParams(name string, handler func(context.Context, json.RawMessage) (any, error)) error {
	if name == "" || len(name) > 64 || handler == nil {
		return errors.New("無效的 P2P 指令")
	}
	switch name {
	case "capabilities.get", "ping", "session.status", "session.disconnect":
		return errors.New("不可覆寫內建指令")
	}
	p.commandsInit()
	p.commands.mu.Lock()
	p.commands.handlers[name] = handler
	p.commands.mu.Unlock()
	p.announceCommands()
	return nil
}
func (p *Peer) RemoteCommands() CommandCapabilities {
	p.commandsInit()
	p.commands.mu.Lock()
	defer p.commands.mu.Unlock()
	c := p.commands.remote
	c.Methods = append([]string(nil), c.Methods...)
	return c
}
func (p *Peer) SupportsCommand(method string) bool {
	c := p.RemoteCommands()
	if c.Version != CommandVersion {
		return false
	}
	for _, m := range c.Methods {
		if m == method {
			return true
		}
	}
	return false
}

// CallCommand 的成功表示收到對端回覆；不把排入 DataChannel 佇列視為執行成功。
func (p *Peer) CallCommand(ctx context.Context, method string) (CommandResponse, error) {
	return p.CallCommandParams(ctx, method, nil)
}
func (p *Peer) CallCommandParams(ctx context.Context, method string, params json.RawMessage) (CommandResponse, error) {
	if len(params) > 8192 || (len(params) > 0 && !json.Valid(params)) {
		return CommandResponse{}, errors.New("指令參數無效或過大")
	}
	if !p.SupportsCommand(method) {
		return CommandResponse{}, ErrCommandUnsupported
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return CommandResponse{}, err
	}
	p.commandsInit()
	p.commands.mu.Lock()
	if len(p.commands.pending) >= 32 {
		p.commands.mu.Unlock()
		return CommandResponse{}, errors.New("P2P 指令等待佇列已滿")
	}
	p.commands.sequence++
	id := p.commands.sequence
	ch := make(chan CommandResponse, 1)
	p.commands.pending[id] = ch
	p.commands.mu.Unlock()
	defer func() { p.commands.mu.Lock(); delete(p.commands.pending, id); p.commands.mu.Unlock() }()
	deadline, _ := ctx.Deadline()
	if err := p.SendControl(Control{Type: "command-request", CommandRequest: &CommandRequest{Version: CommandVersion, ID: id, Method: method, Expires: deadline.UnixMilli(), Params: params}}); err != nil {
		return CommandResponse{}, err
	}
	select {
	case out := <-ch:
		if out.Code != "" {
			return out, fmt.Errorf("%s：%s", out.Code, out.Error)
		}
		return out, nil
	case <-ctx.Done():
		return CommandResponse{}, fmt.Errorf("P2P 指令結果未確認：%w", ctx.Err())
	case <-p.Done():
		return CommandResponse{}, errors.New("P2P 已斷線，指令結果未確認")
	}
}
func (p *Peer) handleCommand(c Control) bool {
	p.commandsInit()
	switch c.Type {
	case "command-capabilities":
		if v := c.CommandCapabilities; v != nil && v.Version == CommandVersion && len(v.Methods) <= 64 {
			for _, m := range v.Methods {
				if len(m) > 64 {
					return true
				}
			}
			p.commands.mu.Lock()
			p.commands.remote = CommandCapabilities{v.Version, append([]string(nil), v.Methods...)}
			p.commands.mu.Unlock()
		}
		return true
	case "command-response":
		r := c.CommandResponse
		if r == nil || r.Version != CommandVersion || len(r.Result) > 16384 || len(r.Error) > 1024 {
			return true
		}
		p.commands.mu.Lock()
		ch := p.commands.pending[r.ID]
		p.commands.mu.Unlock()
		if ch != nil {
			select {
			case ch <- *r:
			default:
			}
		}
		return true
	case "command-request":
		r := c.CommandRequest
		if r == nil || r.ID == 0 {
			return true
		}
		response := CommandResponse{Version: CommandVersion, ID: r.ID}
		reject := func(code, message string) {
			response.Code = code
			response.Error = message
			_ = p.SendControl(Control{Type: "command-response", CommandResponse: &response})
		}
		if r.Version != CommandVersion {
			reject("unsupported_version", "不支援的指令版本")
			return true
		}
		if len(r.Params) > 8192 || len(r.Method) > 64 || r.Expires <= time.Now().UnixMilli() || r.Expires > time.Now().Add(30*time.Second).UnixMilli() {
			reject("expired_or_invalid", "指令已逾時或格式無效")
			return true
		}
		p.commands.mu.Lock()
		if prior, ok := p.commands.received[r.ID]; ok {
			p.commands.mu.Unlock()
			if prior != nil {
				_ = p.SendControl(Control{Type: "command-response", CommandResponse: prior})
			} else {
				reject("in_progress", "指令正在處理")
			}
			return true
		}
		if p.commands.high > 128 && r.ID <= p.commands.high-128 {
			p.commands.mu.Unlock()
			reject("stale_request", "過期的指令序號")
			return true
		}
		if p.commands.active >= 4 {
			p.commands.mu.Unlock()
			reject("busy", "對端忙碌")
			return true
		}
		if r.ID > p.commands.high {
			p.commands.high = r.ID
		}
		for id, prior := range p.commands.received {
			if prior != nil && p.commands.high > 128 && id <= p.commands.high-128 {
				delete(p.commands.received, id)
			}
		}
		p.commands.received[r.ID] = nil
		p.commands.active++
		handler := p.commands.handlers[r.Method]
		p.commands.mu.Unlock()
		request := *r
		go func() {
			ctx, cancel := context.WithDeadline(context.Background(), time.UnixMilli(request.Expires))
			defer cancel()
			go func() {
				select {
				case <-p.Done():
					cancel()
				case <-ctx.Done():
				}
			}()
			var result any
			var err error
			if ctx.Err() != nil {
				response.Code = "expired"
				err = ctx.Err()
			} else {
				switch request.Method {
				case "capabilities.get":
					result = p.localCommands()
				case "ping":
					result = map[string]int64{"remoteTime": time.Now().UnixMilli()}
				case "session.status":
					sent, received := p.TrafficBytes()
					result = map[string]any{"connected": p.Connected(), "sentBytes": sent, "receivedBytes": received}
				case "session.disconnect":
					result = map[string]bool{"accepted": true}
				default:
					if handler == nil {
						response.Code = "unsupported_method"
						err = ErrCommandUnsupported
					} else {
						result, err = handler(ctx, request.Params)
					}
				}
			}
			if err != nil {
				if response.Code == "" {
					response.Code = "failed"
				}
				response.Error = err.Error()
			} else {
				response.Result, err = json.Marshal(result)
				if err != nil || len(response.Result) > 16384 {
					response.Code = "invalid_result"
					response.Error = "回覆無效或過大"
					response.Result = nil
				}
			}
			if len(response.Error) > 1024 {
				response.Error = response.Error[:1024]
			}
			p.commands.mu.Lock()
			p.commands.received[request.ID] = &response
			p.commands.active--
			p.commands.mu.Unlock()
			_ = p.SendControl(Control{Type: "command-response", CommandResponse: &response})
			if request.Method == "session.disconnect" && response.Code == "" {
				time.AfterFunc(200*time.Millisecond, func() { _ = p.Close() })
			}
		}()
		return true
	}
	return false
}
