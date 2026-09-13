package hostsession

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"golang.org/x/image/draw"
	"image"
	"image/png"
	"sync"
	"time"
	"yourdesk/internal/agentvideo"
)

// gate 與送影格階段共用，暫停確認後不再排入影格；已在途影格由 ViewID 丟棄。
// view 狀態由 session 的 displayMu 保護，截圖快取由 gate 保護。
type agentVideo struct {
	gate    sync.Mutex
	view    agentvideo.State
	data    []byte
	token   string
	expires time.Time
}

func newAgentVideo() *agentVideo {
	return &agentVideo{view: agentvideo.State{Mode: "streaming", Region: agentvideo.Full()}}
}

type videoCommandRegistrar interface {
	RegisterCommandParams(string, func(context.Context, json.RawMessage) (any, error)) error
}

func (v *agentVideo) register(peer videoCommandRegistrar, configure func(context.Context, agentvideo.Request) (agentvideo.State, error), capture func(context.Context, agentvideo.State) (image.Image, error), allowed func() bool) {
	apply := func(snapshot bool) func(context.Context, json.RawMessage) (any, error) {
		return func(ctx context.Context, params json.RawMessage) (any, error) {
			var request agentvideo.Request
			if len(params) > 0 {
				if err := json.Unmarshal(params, &request); err != nil {
					return nil, err
				}
			}
			if err := request.Validate(); err != nil {
				return nil, err
			}
			v.gate.Lock()
			defer v.gate.Unlock()
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !allowed() {
				return nil, fmt.Errorf("工作階段已結束")
			}
			state, err := configure(ctx, request)
			if err != nil {
				return nil, err
			}
			v.data = nil
			if !snapshot {
				return state, nil
			}
			img, err := capture(ctx, state)
			if err != nil {
				return nil, err
			}
			img = state.Region.Crop(img)
			w, h := img.Bounds().Dx(), img.Bounds().Dy()
			if max(w, h) > 1280 {
				scale := 1280.0 / float64(max(w, h))
				w = max(1, int(float64(w)*scale))
				h = max(1, int(float64(h)*scale))
				small := image.NewRGBA(image.Rect(0, 0, w, h))
				draw.ApproxBiLinear.Scale(small, small.Bounds(), img, img.Bounds(), draw.Src, nil)
				img = small
			}
			var data bytes.Buffer
			if err = png.Encode(&data, img); err != nil {
				return nil, err
			}
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			if data.Len() > agentvideo.MaxBytes {
				return nil, fmt.Errorf("截圖超過傳輸上限")
			}
			state.Width, state.Height, state.Bytes, state.Token = w, h, data.Len(), rand.Text()
			v.data = data.Bytes()
			v.expires = time.Now().Add(35 * time.Second)
			// 快取只供本連線讀取，不落地保存。
			v.token = state.Token
			return state, nil
		}
	}
	_ = peer.RegisterCommandParams("video.stream", apply(false))
	_ = peer.RegisterCommandParams("video.snapshot", apply(true))
	_ = peer.RegisterCommandParams("video.snapshot.read", func(ctx context.Context, params json.RawMessage) (any, error) {
		var r agentvideo.Read
		if err := json.Unmarshal(params, &r); err != nil {
			return nil, err
		}
		v.gate.Lock()
		defer v.gate.Unlock()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !allowed() {
			return nil, fmt.Errorf("工作階段已結束")
		}
		if time.Now().After(v.expires) {
			v.data = nil
		}
		if r.Token == "" || r.Token != v.token || r.Offset < 0 || r.Offset >= len(v.data) {
			return nil, fmt.Errorf("截圖已過期或讀取範圍無效")
		}
		return append([]byte(nil), v.data[r.Offset:min(r.Offset+agentvideo.ChunkSize, len(v.data))]...), nil
	})
	_ = peer.RegisterCommandParams("video.snapshot.close", func(ctx context.Context, params json.RawMessage) (any, error) {
		var r agentvideo.Read
		if err := json.Unmarshal(params, &r); err != nil {
			return nil, err
		}
		v.gate.Lock()
		defer v.gate.Unlock()
		if r.Token != "" && r.Token == v.token {
			v.data = nil
		}
		return map[string]bool{"closed": true}, nil
	})
}
