package clientui

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"yourdesk/internal/agentremote"
	"yourdesk/internal/agentvideo"
)

type mcpVideo struct {
	Session    string             `json:"session"`
	Mode       string             `json:"mode,omitempty" jsonschema:"paused 暫停擷取及傳送影格，streaming 恢復串流；省略保留目前模式"`
	Region     *agentvideo.Region `json:"region,omitempty" jsonschema:"相對於目前完整螢幕的矩形，x/y/width/height 均為 0～1；套用至截圖及串流"`
	FullScreen bool               `json:"fullScreen,omitempty" jsonschema:"true 清除區域限制，與 region 擇一；兩者省略保留目前視野"`
}

func (s *server) registerVideoTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{Name: "get_video_state", Description: "唯讀查詢已連線桌面的串流模式、視野、輸入就緒及區域／暫停／即時截圖能力；不擷取畫面、不改變模式。capabilitiesKnown=false 時不可推定對端是舊版；changing=true 時 mode/region 為最近確認狀態，先重新設定或截圖再輸入。"}, func(ctx context.Context, r *mcp.CallToolRequest, in mcpSession) (*mcp.CallToolResult, any, error) {
		out, err := s.callRemoteAgent(ctx, mcpAction{Session: in.Session, Action: "video.status"})
		if err != nil {
			return nil, nil, err
		}
		var status map[string]any
		if err = json.Unmarshal(out.Result, &status); err != nil {
			return nil, nil, err
		}
		return nil, status, nil
	})
	for _, name := range []string{"set_video_mode", "snapshot"} {
		name := name
		description := "設定串流模式與視野。paused 保留連線、鍵鼠控制及解碼能力，停止持續傳送影格；streaming 恢復並要求完整影格。切換後先 snapshot 再操作，滑鼠座標以回傳圖片左上角為原點，0～1 對應圖片寬高。區域及暫停需要新版 Host；舊版僅支援全螢幕串流。"
		if name == "snapshot" {
			description = "從遠端系統擷取一張新的 PNG，可選全螢幕或區域；不會自行恢復已暫停的串流。區域與串流共用，region 相對完整螢幕，滑鼠 move/button 相對回傳圖片（0～1），不可再自行加上區域偏移。舊版 Host 的全螢幕請求改用最近收到的影格，回覆 legacy=true、fresh=false；區域及暫停不支援。"
		}
		mcp.AddTool(srv, &mcp.Tool{Name: name, Description: description}, func(ctx context.Context, r *mcp.CallToolRequest, in mcpVideo) (*mcp.CallToolResult, any, error) {
			request := agentvideo.Request{Mode: in.Mode, Region: in.Region, FullScreen: in.FullScreen}
			if err := request.Validate(); err != nil {
				return nil, nil, err
			}
			action := "video.stream"
			if name == "snapshot" {
				action = "video.snapshot"
			}
			params, _ := json.Marshal(request)
			result, err := s.callRemoteAgent(ctx, mcpAction{Session: in.Session, Action: action, Params: params})
			if err != nil {
				return nil, nil, err
			}
			out, err := videoToolResult(result, in.Session, name == "snapshot")
			return out, nil, err
		})
	}
}

// PNG 放在 image content；可程式化資訊統一放 structuredContent，不暴露分塊 token。
func videoToolResult(result agentremote.Response, session string, snapshot bool) (*mcp.CallToolResult, error) {
	var metadata map[string]any
	if err := json.Unmarshal(result.Result, &metadata); err != nil {
		return nil, fmt.Errorf("畫面回覆格式無效：%w", err)
	}
	if metadata == nil {
		return nil, fmt.Errorf("缺少畫面回覆")
	}
	delete(metadata, "token")
	delete(metadata, "bytes")
	metadata["contractVersion"] = 1
	metadata["session"] = session
	legacy, _ := metadata["legacy"].(bool)
	metadata["legacy"] = legacy
	delete(metadata, "fresh")
	if snapshot {
		metadata["fresh"] = !legacy
		metadata["width"] = result.Width
		metadata["height"] = result.Height
	}
	encoded, _ := json.Marshal(metadata)
	out := &mcp.CallToolResult{StructuredContent: metadata, Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}}
	if len(result.Image) > 0 {
		out.Content = append([]mcp.Content{&mcp.ImageContent{Data: result.Image, MIMEType: "image/png"}}, out.Content...)
	}
	return out, nil
}
