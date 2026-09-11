package clientui

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"yourdesk/internal/remotedata"
)

func (s *server) remoteData(ctx context.Context, session, method string, in any) (*mcp.CallToolResult, any, error) {
	params, err := json.Marshal(in)
	if err != nil {
		return nil, nil, err
	}
	out, err := s.callRemoteAgent(ctx, mcpAction{Session: session, Action: method, Params: params})
	if err != nil {
		return nil, nil, err
	}
	var result any
	if err = json.Unmarshal(out.Result, &result); err != nil {
		return nil, nil, err
	}
	return nil, result, nil
}
func (s *server) registerRemoteDataTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{Name: "get_remote_filesystem", Description: "查詢已授權遠端電腦的檔案搜尋根目錄與讀取限制；不是本機檔案系統。需要兩端支援新 P2P 指令，未登入的 root Host 不開放。"}, func(ctx context.Context, r *mcp.CallToolRequest, in mcpSession) (*mcp.CallToolResult, any, error) {
		return s.remoteData(ctx, in.Session, "files.roots", struct{}{})
	})
	mcp.AddTool(srv, &mcp.Tool{Name: "search_remote_files", Description: "在遠端使用者家目錄搜尋檔名（query，不區分大小寫），可加 content 比對 UTF-8 文字內容（區分大小寫，每檔前 64 KiB）。path 可用根目錄內絕對或相對路徑；limit 預設/最多 20，depth 預設 4、最多 8。truncated 表示結果不完整，可縮小範圍再查。跳過符號連結。"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct {
		Session string `json:"session"`
		remotedata.Search
	}) (*mcp.CallToolResult, any, error) { return s.remoteData(ctx, in.Session, "files.search", in.Search) })
	mcp.AddTool(srv, &mcp.Tool{Name: "read_remote_file", Description: "分塊讀取遠端家目錄內一般檔案，length 預設/最多 4096 bytes；offset 從 0 起。data 為 Base64 原始資料，使用 nextOffset 續讀直到 eof；請檢查 size/modified 避免拼接已變更檔案。不支援符號連結或特殊裝置。"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct {
		Session string `json:"session"`
		remotedata.Read
	}) (*mcp.CallToolResult, any, error) { return s.remoteData(ctx, in.Session, "files.read", in.Read) })
	mcp.AddTool(srv, &mcp.Tool{Name: "run_remote_shell", Description: "在已授權的遠端電腦執行明確指定的 command；macOS 使用 /bin/sh，Windows 使用 cmd.exe。權限與遠端 Host 帳號相同，可修改檔案與系統狀態，並非檔案唯讀介面。directory 預設遠端家目錄；timeoutMs 預設/最多 3000、最少 100。回傳合併輸出（最多 2048 bytes，非 UTF-8 以替代字元顯示）、exitCode、timedOut、truncated；每次獨立執行，逾時或結束後回收所管理的子程序。禁止用於長駐服務；未登入 root Host 不開放。"}, func(ctx context.Context, r *mcp.CallToolRequest, in struct {
		Session string `json:"session"`
		remotedata.Shell
	}) (*mcp.CallToolResult, any, error) { return s.remoteData(ctx, in.Session, "shell.run", in.Shell) })
}
