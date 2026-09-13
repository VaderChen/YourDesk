package hostsession

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
	"yourdesk/internal/agentvideo"
)

type videoCommands map[string]func(context.Context, json.RawMessage) (any, error)

func (c videoCommands) RegisterCommandParams(name string, handler func(context.Context, json.RawMessage) (any, error)) error {
	c[name] = handler
	return nil
}
func TestAgentSnapshotSmoke(t *testing.T) {
	v := newAgentVideo()
	commands := videoCommands{}
	captures := 0
	allowed := true
	v.register(commands, func(ctx context.Context, r agentvideo.Request) (agentvideo.State, error) {
		v.view.ViewID++
		if r.Mode != "" {
			v.view.Mode = r.Mode
		}
		if r.Region != nil {
			v.view.Region = *r.Region
		}
		if r.FullScreen {
			v.view.Region = agentvideo.Full()
		}
		v.view.DisplayCount = 1
		return v.view, nil
	}, func(ctx context.Context, state agentvideo.State) (image.Image, error) {
		captures++
		img := image.NewRGBA(image.Rect(0, 0, 40, 20))
		img.Set(10, 5, color.RGBA{uint8(captures), 0, 0, 255})
		return img, nil
	}, func() bool { return allowed })
	call := func(name, params string) any {
		t.Helper()
		out, err := commands[name](context.Background(), json.RawMessage(params))
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	state := call("video.stream", `{"mode":"paused","region":{"x":0.25,"y":0.25,"width":0.5,"height":0.5}}`).(agentvideo.State)
	if state.Mode != "paused" || captures != 0 {
		t.Fatal("暫停不應擷取畫面")
	}
	for i := 1; i <= 2; i++ {
		state = call("video.snapshot", `{}`).(agentvideo.State)
		if state.Mode != "paused" || state.Width != 20 || state.Height != 10 || captures != i {
			t.Fatalf("截圖未保留模式/範圍 %+v", state)
		}
		params, _ := json.Marshal(agentvideo.Read{Token: state.Token})
		data := call("video.snapshot.read", string(params)).([]byte)
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		red, _, _, _ := img.At(0, 0).RGBA()
		if red != uint32(i)*257 {
			t.Fatal("截圖使用舊畫面或裁切偏移")
		}
		call("video.snapshot.close", string(params))
		if _, err = commands["video.snapshot.read"](context.Background(), params); err == nil {
			t.Fatal("關閉後仍能讀取")
		}
	}
	state = call("video.stream", `{"mode":"streaming","fullScreen":true}`).(agentvideo.State)
	if state.Mode != "streaming" || state.Region != agentvideo.Full() {
		t.Fatal("未恢復全螢幕串流")
	}
	allowed = false
	if _, err := commands["video.snapshot"](context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("失效工作階段仍可擷取")
	}
	allowed = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := commands["video.stream"](ctx, json.RawMessage(`{}`)); err == nil {
		t.Fatal("執行過期指令")
	}
}
func TestAgentSnapshotLimits(t *testing.T) {
	v := newAgentVideo()
	c := videoCommands{}
	v.register(c, func(ctx context.Context, r agentvideo.Request) (agentvideo.State, error) { return v.view, nil }, func(context.Context, agentvideo.State) (image.Image, error) {
		return image.NewRGBA(image.Rect(0, 0, 1920, 1080)), nil
	}, func() bool { return true })
	result, err := c["video.snapshot"](context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	state := result.(agentvideo.State)
	if state.Width != 1280 || state.Height != 720 {
		t.Fatalf("未限制截圖大小 %+v", state)
	}
	v.data = make([]byte, agentvideo.ChunkSize+1)
	for _, offset := range []int{-1, len(v.data)} {
		params, _ := json.Marshal(agentvideo.Read{Token: state.Token, Offset: offset})
		if _, err = c["video.snapshot.read"](context.Background(), params); err == nil {
			t.Fatal("接受越界讀取")
		}
	}
	params, _ := json.Marshal(agentvideo.Read{Token: state.Token})
	result, err = c["video.snapshot.read"](context.Background(), params)
	if err != nil || len(result.([]byte)) != agentvideo.ChunkSize {
		t.Fatal("分塊大小錯誤")
	}
	v.expires = time.Now().Add(-time.Second)
	if _, err = c["video.snapshot.read"](context.Background(), params); err == nil || v.data != nil {
		t.Fatal("未清除過期截圖")
	}
}
