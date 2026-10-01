// 手機 LAN Smoke 的協定 Host：只傳測試影像與回音，不擷取桌面、不執行 Shell 或注入輸入。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"yourdesk/internal/p2p"
	"yourdesk/internal/security"
	"yourdesk/internal/signaling"
)

func frame(fill color.RGBA) []byte {
	canvas := image.NewRGBA(image.Rect(0, 0, 640, 360))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{0, 255, 0, 255}), image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(8, 8, 632, 352), image.NewUniform(fill), image.Point{}, draw.Src)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, canvas, &jpeg.Options{Quality: 90}); err != nil {
		log.Fatal(err)
	}
	return encoded.Bytes()
}

func main() {
	listen := flag.String("listen", "127.0.0.1:47829", "測試裝置可達的本機 IP:port")
	secretFile := flag.String("secret-file", "", "臨時測試密碼檔")
	flag.Parse()
	password, err := os.ReadFile(*secretFile)
	if err != nil {
		log.Fatal(err)
	}
	secret, err := security.DecodeSecret(strings.TrimSpace(string(password)))
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	normal := frame(color.RGBA{0, 100, 200, 255})
	acknowledged := frame(color.RGBA{220, 20, 20, 255})
	log.Printf("手機協定測試 Host：%s", *listen)
	err = signaling.ListenDirect(ctx, *listen, secret, func(session context.Context, client *signaling.Client) {
		var receivedText atomic.Bool
		desktop := newDesktopSmoke()
		peer, err := p2p.NewHost(session, client, func(control p2p.Control) {
			desktop.control(control)
			if control.Type == "text" && control.Text == "Android Smoke 中文" {
				receivedText.Store(true)
			}
		})
		if err != nil {
			log.Printf("建立測試連線失敗：%v", err)
			return
		}
		defer peer.Close()
		attachAudio(session, peer)
		desktop.attach(peer)
		register := func(name string, handler func(context.Context, json.RawMessage) (any, error)) {
			if err := peer.RegisterCommandParams(name, handler); err != nil {
				log.Fatal(err)
			}
		}
		register("video.keyframe", func(context.Context, json.RawMessage) (any, error) { return map[string]bool{"accepted": true}, nil })
		register("input.text-capabilities", func(context.Context, json.RawMessage) (any, error) { return map[string]bool{"supported": true}, nil })
		var mu sync.Mutex
		var sequence uint64
		var output []byte
		ended := false
		register("terminal.open", func(context.Context, json.RawMessage) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			sequence = 1
			output = []byte("YourDesk 協定 Smoke\r\n")
			ended = false
			return map[string]bool{"opened": true}, nil
		})
		register("terminal.write", func(_ context.Context, raw json.RawMessage) (any, error) {
			var input struct {
				Data []byte `json:"data"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			mu.Lock()
			defer mu.Unlock()
			sequence++
			output = append([]byte("回音："), input.Data...)
			return map[string]bool{"accepted": true}, nil
		})
		register("terminal.read", func(_ context.Context, raw json.RawMessage) (any, error) {
			var input struct {
				Ack uint64 `json:"ack"`
			}
			if err := json.Unmarshal(raw, &input); err != nil {
				return nil, err
			}
			mu.Lock()
			defer mu.Unlock()
			data := []byte{}
			if input.Ack < sequence {
				data = append(data, output...)
			}
			return map[string]any{"sequence": sequence, "data": data, "ended": ended}, nil
		})
		register("terminal.resize", func(context.Context, json.RawMessage) (any, error) { return map[string]bool{"accepted": true}, nil })
		register("terminal.close", func(context.Context, json.RawMessage) (any, error) {
			mu.Lock()
			defer mu.Unlock()
			ended = true
			return map[string]bool{"closed": true}, nil
		})
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		var index uint64
		for {
			select {
			case <-session.Done():
				return
			case <-peer.Done():
				log.Print("手機測試連線已結束")
				return
			case <-ticker.C:
				if !peer.Connected() {
					continue
				}
				payload := normal
				if receivedText.Load() {
					payload = acknowledged
				}
				index++
				desktop.send(peer, index, payload)
			}
		}
	})
	if err != nil {
		log.Fatal(err)
	}
}
