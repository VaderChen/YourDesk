package signaling

import "context"

// HostCapabilities 是公開的功能摘要，不含帳號、檔案、密碼或 Shell 路徑。
// nil 表示舊版未回報，不能當成所有能力皆為 false。
type HostCapabilities struct {
	Schema    int    `json:"schema"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Version   string `json:"version"`
	Desktop   bool   `json:"desktop"`
	Terminal  bool   `json:"terminal"`
	Clipboard bool   `json:"clipboard"`
}
type hostCapabilitiesKey struct{}

func WithHostCapabilities(ctx context.Context, c HostCapabilities) context.Context {
	return context.WithValue(ctx, hostCapabilitiesKey{}, c)
}
func joinPayload(ctx context.Context, role Role) any {
	if role != RoleHost {
		return nil
	}
	if c, ok := ctx.Value(hostCapabilitiesKey{}).(HostCapabilities); ok {
		return struct {
			Capabilities HostCapabilities `json:"capabilities"`
		}{c}
	}
	return nil
}
