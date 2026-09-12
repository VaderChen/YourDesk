package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"yourdesk/internal/agentremote"
	"yourdesk/internal/optimization"
	"yourdesk/internal/security"
)

const uiEventPrefix = "YOURDESK_UI_EVENT "

func emitUIEvent(event, message string) {
	data, _ := json.Marshal(map[string]string{"event": event, "message": message})
	fmt.Fprintln(os.Stdout, uiEventPrefix+string(data))
}

// 密碼只經標準輸入傳入，不寫入站台資料或診斷輸出。
type passwordValue struct {
	secret string
	err    error
}

type passwordInput struct {
	done     chan struct{}
	agent    chan agentremote.Request
	values   chan passwordValue
	start    sync.Once
	attempts int
}

func newPasswordInput() *passwordInput {
	return &passwordInput{done: make(chan struct{}), values: make(chan passwordValue, 1), agent: make(chan agentremote.Request, 8)}
}

func (p *passwordInput) read(ctx context.Context, prompt bool) ([]byte, error) {
	p.start.Do(func() {
		go func() {
			defer close(p.values)
			defer close(p.done)
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Buffer(make([]byte, 1024), 65536)
			for scanner.Scan() {
				var envelope struct {
					Agent        *agentremote.Request `json:"agent"`
					ProbeFPS     *int                 `json:"probeFPS"`
					Optimization *optimization.Policy `json:"optimization"`
				}
				if json.Unmarshal(scanner.Bytes(), &envelope) == nil && envelope.Optimization != nil {
					// 本機父程序的控制訊息不得進入密碼重試佇列。
					if optimization.Apply(*envelope.Optimization) {
						slog.Info("觀看端已接收本機解碼策略", "combinations", len(envelope.Optimization.Decoders))
					}
					continue
				}
				if json.Unmarshal(scanner.Bytes(), &envelope) == nil && envelope.ProbeFPS != nil {
					if *envelope.ProbeFPS == 30 {
						fpsProbeUntil.Store(time.Now().Add(30 * time.Second).UnixMilli())
					} else if *envelope.ProbeFPS == 0 {
						fpsProbeUntil.Store(0)
					}
					continue
				}
				if json.Unmarshal(scanner.Bytes(), &envelope) == nil && envelope.Agent != nil {
					select {
					case p.agent <- *envelope.Agent:
					default:
						data, _ := json.Marshal(agentremote.Response{ID: envelope.Agent.ID, Error: "操作佇列已滿"})
						fmt.Fprintln(os.Stdout, agentremote.Prefix+string(data))
					}
					continue
				}
				var value struct {
					Secret string `json:"secret"`
				}
				err := json.Unmarshal(scanner.Bytes(), &value)
				if err != nil {
					err = errors.New("密碼輸入格式無效")
				}
				select {
				case p.values <- passwordValue{secret: value.Secret, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	})
	message := "此裝置需要連線密碼，請輸入被控端提供的密碼。"
	if p.attempts > 0 {
		message = "連線密碼不正確，請重新輸入。"
	}
	for {
		if prompt {
			emitUIEvent("password-required", message)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case value, ok := <-p.values:
			if !ok {
				return nil, errors.New("密碼輸入已取消")
			}
			if value.err != nil {
				message = value.err.Error()
				continue
			}
			secret, err := security.DecodeSecret(value.secret)
			if err != nil {
				message = err.Error()
				continue
			}
			p.attempts++
			return secret, nil
		}
	}
}

func (p *passwordInput) resolver() func(context.Context) ([]byte, error) {
	return func(ctx context.Context) ([]byte, error) { return p.read(ctx, true) }
}
