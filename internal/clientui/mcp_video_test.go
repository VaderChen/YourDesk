package clientui

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"yourdesk/internal/agentremote"
)

func TestMCPVideoSchemaSmoke(t *testing.T) {
	app := &server{children: map[string]*process{}}
	srv := app.mcpServer()
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "video-smoke", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, tool := range list.Tools {
		found[tool.Name] = true
	}
	for _, name := range []string{"connect", "get_video_state", "snapshot", "set_video_mode", "remote_action"} {
		if !found[name] {
			t.Fatal("缺少 MCP 工具", name)
		}
	}
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"snapshot", map[string]any{"session": "quick", "region": map[string]float64{"x": .9, "y": 0, "width": .5, "height": 1}}},
		{"set_video_mode", map[string]any{"session": "quick", "mode": "invalid"}},
		{"snapshot", map[string]any{"session": "quick", "fullScreen": true, "region": map[string]float64{"x": 0, "y": 0, "width": 1, "height": 1}}},
		{"connect", map[string]any{"room": "test", "terminal": true, "videoMode": "paused"}},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Fatal("接受無效參數", tc)
		}
	}
}

func TestVideoOutputContract(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		state := map[string]any{"viewID": 7, "mode": "paused", "region": map[string]float64{"x": .25, "y": .25, "width": .5, "height": .5}, "display": 0, "displayCount": 1, "legacy": legacy, "token": "private-token", "bytes": 123, "fresh": false}
		data, _ := json.Marshal(state)
		response := agentremote.Response{Result: data, Image: []byte{1, 2, 3}, Width: 320, Height: 180}
		result, err := videoToolResult(response, "quick", true)
		if err != nil {
			t.Fatal(err)
		}
		metadata := result.StructuredContent.(map[string]any)
		if metadata["contractVersion"] != 1 || metadata["session"] != "quick" || metadata["fresh"] != !legacy || metadata["width"] != 320 {
			t.Fatalf("契約欄位不符 %+v", metadata)
		}
		if _, ok := metadata["token"]; ok {
			t.Fatal("暴露傳輸 token")
		}
		if _, ok := metadata["bytes"]; ok {
			t.Fatal("暴露分塊資訊")
		}
		if len(result.Content) != 2 {
			t.Fatal("缺少圖片或 JSON 內容")
		}
		if _, ok := result.Content[0].(*mcp.ImageContent); !ok {
			t.Fatal("未回傳 MCP image")
		}
		text := result.Content[1].(*mcp.TextContent).Text
		encoded, _ := json.Marshal(metadata)
		if text != string(encoded) {
			t.Fatal("文字與結構化內容不一致")
		}
		response.Image = nil
		result, err = videoToolResult(response, "quick", false)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := result.StructuredContent.(map[string]any)["fresh"]; ok {
			t.Fatal("設定模式不可宣稱取得圖片")
		}
	}
}
