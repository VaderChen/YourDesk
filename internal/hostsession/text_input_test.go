package hostsession

import (
	"encoding/json"
	"strings"
	"testing"
	"yourdesk/internal/input"
	"yourdesk/internal/p2p"
)

type committedTextController struct {
	input.Controller
	texts []string
}

func (c *committedTextController) Text(text string) error {
	c.texts = append(c.texts, text)
	return nil
}

func TestTextControlDispatch(t *testing.T) {
	var control p2p.Control
	if err := json.Unmarshal([]byte(`{"type":"text","text":"中文😀"}`), &control); err != nil {
		t.Fatal(err)
	}
	c := &committedTextController{}
	handleControl(c, control)
	if len(c.texts) != 1 || c.texts[0] != "中文😀" {
		t.Fatalf("Unicode control was not dispatched: %q", c.texts)
	}
	for _, text := range []string{strings.Repeat("x", input.MaxTextBytes+1), string([]byte{0xff}), ""} {
		handleControl(c, p2p.Control{Type: "text", Text: text})
	}
	if len(c.texts) != 1 {
		t.Fatal("invalid or empty text reached input injection")
	}
}
