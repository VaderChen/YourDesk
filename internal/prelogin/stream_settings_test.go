package prelogin

import (
	"errors"
	"strings"
	"testing"
)

func TestStreamSettingsSmoke(t *testing.T) {
	original := Config{Room: "keep-room", Signal: "wss://keep.invalid", Secret: "keep-secret", Codec: "auto", CodecGoal: "balanced", Direct: true, Tailcat: true}
	current := original
	calls := 0
	save := func(next Config) error {
		calls++
		if next.Room != original.Room || next.Signal != original.Signal || next.Secret != original.Secret || next.Direct != original.Direct || next.Tailcat != original.Tailcat {
			t.Fatal("編碼更新改到了配對或網路設定")
		}
		return nil
	}
	settings := StreamSettings{Codec: "hardware-hevc", CodecGoal: "bandwidth"}
	if changed, err := applyStreamSettings(&current, settings, save); err != nil || !changed || calls != 1 || current.Codec != settings.Codec || current.CodecGoal != settings.CodecGoal {
		t.Fatalf("未套用編碼：%+v，%v", current, err)
	}
	if changed, err := applyStreamSettings(&current, settings, save); err != nil || changed || calls != 1 {
		t.Fatal("相同請求不應再次持久化或重啟")
	}
	before := current
	if changed, err := applyStreamSettings(&current, StreamSettings{Codec: "invalid"}, save); err == nil || changed || current != before || calls != 1 {
		t.Fatal("非法編碼應在持久化前拒絕")
	}
	if changed, err := applyStreamSettings(&current, StreamSettings{Codec: "auto"}, func(Config) error { return errors.New("disk full") }); err == nil || changed || current != before {
		t.Fatal("儲存失敗不應改變執行中的編碼")
	}
	for _, codec := range []string{"auto", "hardware-h264", "hardware-hevc", "hardware-av1", "software-av1", "hardware-jpeg", "software-jpeg"} {
		if err := (StreamSettings{Codec: codec}).Validate(); err != nil {
			t.Fatalf("服務拒絕介面支援的編碼 %s：%v", codec, err)
		}
	}
}

func TestStreamSettingsReplySmoke(t *testing.T) {
	for _, payload := range []string{"", `{}`, `false`, `{"ok":false}`, `{"error":"拒絕"}`} {
		if readStreamSettingsReply(strings.NewReader(payload)) == nil {
			t.Fatalf("未確認服務成功就接受回應：%s", payload)
		}
	}
	if err := readStreamSettingsReply(strings.NewReader(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
}
