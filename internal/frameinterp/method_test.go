package frameinterp

import (
	"context"
	"image"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestAppleFrameBufferValidation(t *testing.T) {
	parent := image.NewRGBA(image.Rect(0, 0, 16, 16))
	sub := parent.SubImage(image.Rect(3, 4, 12, 16)).(*image.RGBA)
	if !validAppleFramePair(sub, sub) {
		t.Fatal("有效子影像被拒絕")
	}
	if validAppleFramePair(nil, sub) {
		t.Fatal("接受 nil")
	}
	truncated := *sub
	truncated.Pix = truncated.Pix[:len(truncated.Pix)-20]
	if validAppleFramePair(sub, &truncated) {
		t.Fatal("接受不完整末列")
	}
	badStride := *sub
	badStride.Stride = 1
	if validAppleFramePair(sub, &badStride) {
		t.Fatal("接受過短 stride")
	}
	if validAppleFramePair(image.NewRGBA(image.Rect(0, 0, 1, 1)), image.NewRGBA(image.Rect(0, 0, 1, 1))) {
		t.Fatal("接受過小影格")
	}
}

func TestAppleUnavailableBeforeMacOS27(t *testing.T) {
	if runtime.GOOS == "darwin" {
		data, err := exec.Command("sw_vers", "-productVersion").Output()
		if err != nil {
			t.Fatal(err)
		}
		major, err := strconv.Atoi(strings.Split(strings.TrimSpace(string(data)), ".")[0])
		if err != nil {
			t.Fatal(err)
		}
		if major >= 27 {
			t.Skip("此檢查驗證 macOS 27 以下的停用路徑")
		}
	}
	if AppleSupported() {
		t.Fatal("低版本／非 Mac 不得啟用 Apple 補幀")
	}
	if ResolveMethod("") == "apple" {
		t.Fatal("自動模式誤選 Apple")
	}
	if ok, reason := MethodStatus("apple"); ok || !strings.Contains(reason, "macOS 27") {
		t.Fatalf("不可用狀態：%v %s", ok, reason)
	}
	frame := image.NewRGBA(image.Rect(0, 0, 4, 4))
	if _, err := MidpointWithMethod(context.Background(), frame, frame, "apple"); err == nil {
		t.Fatal("低版本仍呼叫 Apple 推論")
	}
}
