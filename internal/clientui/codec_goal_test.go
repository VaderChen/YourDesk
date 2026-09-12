package clientui

import (
	"encoding/json"
	"testing"
)

func TestCodecGoalPreference(t *testing.T) {
	p := Preferences{Language: "auto", Theme: "default", SourceFPSLimit: 20, BitrateLimitMbps: 12, KeyframeInterval: 10}
	for _, goal := range []string{"", "balanced", "low-latency", "bandwidth"} {
		p.CodecGoal = goal
		if err := p.validate(); err != nil {
			t.Fatalf("%s: %v", goal, err)
		}
		data, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var saved Preferences
		if err = json.Unmarshal(data, &saved); err != nil || saved.CodecGoal != goal {
			t.Fatal("偏好未完整保存")
		}
	}
	p.CodecGoal = "invalid"
	if p.validate() == nil {
		t.Fatal("接受未知串流偏好")
	}
}
