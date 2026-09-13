package signaling

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// 對獨立執行的新版 Server 實測；URL 必須為測試環境，憑證由 YOURDESK_TLS_CA 提供。
func TestV2ServerIntegration(t *testing.T) {
	base := os.Getenv("YOURDESK_V2_SMOKE_URL")
	if base == "" {
		t.Skip("需指定獨立 Server 測試 URL")
	}
	for _, ht := range []string{"wss", "https"} {
		for _, vt := range []string{"wss", "https"} {
			t.Run(ht+"-"+vt, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				defer cancel()
				address := func(mode string) string {
					if mode == "wss" {
						return "wss" + strings.TrimPrefix(base, "https") + "/ws"
					}
					return base
				}
				room := "migration-" + ht + "-" + vt
				key := []byte("local-smoke-only")
				host, err := Dial(ctx, address(ht), room, RoleHost, key)
				if err != nil {
					t.Fatal(err)
				}
				defer host.Close()
				viewer, err := Dial(ctx, address(vt), room, RoleViewer, key)
				if err != nil {
					t.Fatal(err)
				}
				defer viewer.Close()
				for _, c := range []*Client{host, viewer} {
					if c.heartbeat.Protocol != heartbeatV2 {
						t.Fatalf("未真正接受 v2：%+v", c.heartbeat)
					}
				}
				if err := host.Send(ctx, KindOffer, map[string]string{"sdp": "integration-offer"}); err != nil {
					t.Fatal(err)
				}
				offer, err := viewer.Receive(ctx)
				if err != nil || offer.Kind != KindOffer {
					t.Fatalf("Offer: %v %v", offer.Kind, err)
				}
				if err := viewer.Send(ctx, KindAnswer, map[string]string{"sdp": "integration-answer"}); err != nil {
					t.Fatal(err)
				}
				answer, err := host.Receive(ctx)
				if err != nil || answer.Kind != KindAnswer {
					t.Fatalf("Answer: %v %v", answer.Kind, err)
				}
			})
		}
	}
}
