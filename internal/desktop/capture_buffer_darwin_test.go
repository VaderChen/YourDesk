//go:build darwin && cgo

package desktop

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeCaptureBufferOwnership(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "capture-buffer-test")
	cmd := exec.CommandContext(ctx, "xcrun", "clang", "testdata/capture_buffer_darwin.m", "-o", binary,
		"-framework", "Foundation", "-framework", "ScreenCaptureKit", "-framework", "CoreMedia",
		"-framework", "CoreVideo", "-framework", "CoreGraphics")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile native capture test: %v\n%s", err, output)
	}
	if output, err := exec.CommandContext(ctx, binary).CombinedOutput(); err != nil {
		t.Fatalf("native capture ownership: %v\n%s", err, output)
	}
}
