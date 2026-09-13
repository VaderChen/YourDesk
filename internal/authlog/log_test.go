package authlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnosticBounds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YOURDESK_AUTH_LOG_DIR", dir)
	old := Enabled
	Enabled = "1"
	if err := SetDebug(true); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if file != nil {
			file.Close()
		}
		file = nil
		written = 0
		stackCount = 0
		Enabled = old
	}()
	Start("test", "viewer")
	Stacks()
	Stacks()
	Stacks()
	if stackCount != 2 {
		t.Fatal("stack limit")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	before, _ := os.ReadFile(files[0])
	written = 5 << 20
	Event("must_not_write", nil)
	after, _ := os.ReadFile(files[0])
	if string(before) != string(after) {
		t.Fatal("file exceeded limit")
	}
	var first map[string]any
	for i, b := range before {
		if b == '\n' {
			if err := json.Unmarshal(before[:i], &first); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if first["event"] != "startup" {
		t.Fatal(first)
	}
}

func TestRuntimeDebugGate(t *testing.T) {
	t.Setenv("YOURDESK_AUTH_LOG_DIR", t.TempDir())
	mu.Lock()
	if file != nil {
		file.Close()
	}
	file = nil
	written = 0
	mu.Unlock()
	defer func() {
		mu.Lock()
		if file != nil {
			file.Close()
		}
		file = nil
		mu.Unlock()
	}()
	Start("runtime-test", "host")
	if file != nil {
		t.Fatal("disabled startup created log")
	}
	if err := SetDebug(true); err != nil {
		t.Fatal(err)
	}
	Event("enabled", nil)
	if file == nil {
		t.Fatal("runtime enable did not create log")
	}
	before := written
	if err := SetDebug(false); err != nil {
		t.Fatal(err)
	}
	Event("disabled", nil)
	Stacks()
	if written != before {
		t.Fatal("disabled runtime wrote log")
	}
}
