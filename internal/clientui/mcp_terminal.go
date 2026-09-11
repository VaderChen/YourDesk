package clientui

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpTerminal struct {
	Session  string `json:"session" jsonschema:"connect terminal=true 後，get_status 回傳的 session"`
	Instance string `json:"instance" jsonschema:"同一筆 get_status 回傳的 instance；重連後必須重新取得"`
	Action   string `json:"action" jsonschema:"open、read、write、resize、close"`
	Columns  int    `json:"columns,omitempty" jsonschema:"open/resize 的欄數，20～500；預設 80"`
	Rows     int    `json:"rows,omitempty" jsonschema:"open/resize 的列數，5～300；預設 24"`
	Text     string `json:"text,omitempty" jsonschema:"write 的原始 UTF-8 輸入，最多 2048 bytes；Enter 使用 \r，Ctrl+C 使用 \u0003"`
	Sequence uint64 `json:"sequence,omitempty" jsonschema:"write 輸入序號，從 1 依序增加；同一筆輸入重試沿用序號及內容"`
	Ack      uint64 `json:"ack,omitempty" jsonschema:"read 已處理的上一筆輸出序號，首次 0；未確認會重傳相同輸出"`
}

func (s *server) registerTerminalTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{Name: "get_site_capabilities", Description: "查詢已儲存站台的在線狀態與桌面、命令列、剪貼簿能力，結果以站台 ID 為鍵。舊版、IP 直連或查詢失敗可能未知；缺少能力不代表不支援。這是 Server 保存的公告，實際連線仍會確認支援。兩種模式皆可用時，Shell 工作優先命令列，需要 GUI 或截圖時使用桌面。"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
		out, err := s.mcpAPI(ctx, "GET", "/api/presence", nil)
		return nil, out, err
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "remote_terminal", Description: "操作 MCP 建立的互動命令列。先 connect terminal=true，等待 connected，再 open；以 read 持續收取 Base64 原始輸出（含 ANSI 控制碼），write 傳送輸入，resize 調整尺寸，close 結束命令列與連線。工作目錄與變數會保留；命令使用遠端帳號權限。read 的空資料不表示結束，請依 ended 判斷並讀完資料。收到終端機查詢碼時須透過 write 回覆（例如 ESC[6n 的游標位置回覆 ESC[1;1R）。輸入逾時重試須沿用序號與內容，不可當成新指令重送。不能操作使用者視窗的命令列。"}, func(ctx context.Context, r *mcp.CallToolRequest, in mcpTerminal) (*mcp.CallToolResult, any, error) {
		switch in.Action {
		case "open", "read", "write", "resize", "close":
		default:
			return nil, nil, fmt.Errorf("不支援的終端機操作")
		}
		if in.Columns == 0 {
			in.Columns = 80
		}
		if in.Rows == 0 {
			in.Rows = 24
		}
		if (in.Action == "open" || in.Action == "resize") && (in.Columns < 20 || in.Columns > 500 || in.Rows < 5 || in.Rows > 300) {
			return nil, nil, fmt.Errorf("終端機大小無效")
		}
		if in.Action == "write" && (in.Sequence == 0 || len(in.Text) > 2048 || !utf8.ValidString(in.Text)) {
			return nil, nil, fmt.Errorf("輸入須為最多 2048 bytes 的 UTF-8 文字，且序號必須從 1 開始")
		}
		s.mu.Lock()
		p := s.children[in.Session]
		valid := p != nil && p.mcpOwned && p.terminalConnection && in.Instance != "" && p.terminalInstance == in.Instance
		for _, child := range s.children {
			if child.terminalOwner == p && p != nil {
				valid = false
			}
		}
		s.mu.Unlock()
		if !valid {
			return nil, nil, fmt.Errorf("請使用 MCP 建立的命令列連線及最新 instance；不能共用已開啟的終端機視窗")
		}
		params, err := json.Marshal(map[string]any{"columns": in.Columns, "rows": in.Rows, "data": []byte(in.Text), "sequence": in.Sequence, "ack": in.Ack})
		if err != nil {
			return nil, nil, err
		}
		out, err := s.callRemoteAgent(ctx, mcpAction{expectedProcess: p, Session: in.Session, Action: "terminal." + in.Action, Params: params})
		if err != nil {
			return nil, nil, err
		}
		var result any
		err = json.Unmarshal(out.Result, &result)
		return nil, result, err
	})
}
