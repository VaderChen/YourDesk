package clientui

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestHostSecretPipeHelper(t *testing.T) {
	if os.Getenv("YOURDESK_AUTH_SMOKE_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	var request struct {
		Secret string `json:"secret"`
	}
	if !scanner.Scan() || json.Unmarshal(scanner.Bytes(), &request) != nil || request.Secret != "local-smoke-密碼 " {
		os.Exit(3)
	}
	os.Exit(0)
}
func TestHostSecretPipeSmoke(t *testing.T) {
	t.Setenv("YOURDESK_AUTH_SMOKE_HELPER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := &server{children: make(map[string]*process)}
	s.mu.Lock()
	err = s.start("host", "", executable, []string{"-test.run=^TestHostSecretPipeHelper$"})
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	child := s.children["host"]
	err = sendProcessSecret(child, "local-smoke-密碼 ")
	s.mu.Unlock()
	if err != nil {
		child.cmd.Process.Kill()
		t.Fatal(err)
	}
	select {
	case <-child.done:
	case <-time.After(5 * time.Second):
		child.cmd.Process.Kill()
		t.Fatal("host password handoff timed out")
	}
	if !child.cmd.ProcessState.Success() {
		t.Fatal("host did not receive the exact password")
	}
}
