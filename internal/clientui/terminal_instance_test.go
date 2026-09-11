package clientui

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTerminalRejectsStaleWindow(t *testing.T) {
	s := &server{children: map[string]*process{"viewer:one": {terminalConnection: true, terminalInstance: "new-instance"}}}
	for _, action := range []string{"write", "close", "disconnect"} {
		r := httptest.NewRequest("POST", "/api/terminal", strings.NewReader(`{"session":"viewer:one","instance":"old-instance","action":"`+action+`","params":{}}`))
		w := httptest.NewRecorder()
		s.terminalAction(w, r)
		if w.Code != 400 {
			t.Fatalf("舊視窗 %s 未被拒絕: %d", action, w.Code)
		}
	}
}
