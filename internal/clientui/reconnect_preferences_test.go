package clientui

import (
	"encoding/json"
	"testing"
)

func TestReconnectPreferencesExclusive(t *testing.T) {
	var old Preferences
	if err := json.Unmarshal([]byte(`{"closeWindowOnDisconnect":true}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.AutoReconnect {
		t.Fatal("legacy preferences enabled experimental reconnect")
	}
	conflicting := Preferences{AutoReconnect: true, CloseWindowOnDisconnect: true}
	if conflicting.validate() == nil {
		t.Fatal("mutually exclusive settings accepted")
	}
}
