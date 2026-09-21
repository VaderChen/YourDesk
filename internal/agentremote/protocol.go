// Package agentremote 定義 APP 與其遠端顯示子程序之間的私有操作協定。
package agentremote

import "encoding/json"

type Request struct {
	Params  json.RawMessage `json:"params,omitempty"`
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	X       float64         `json:"x,omitempty"`
	Y       float64         `json:"y,omitempty"`
	Button  int             `json:"button,omitempty"`
	Down    bool            `json:"down,omitempty"`
	Key     string          `json:"key,omitempty"`
	Text    string          `json:"text,omitempty"`
	Delta   float64         `json:"delta,omitempty"`
	Display int             `json:"display,omitempty"`
	Expires int64           `json:"expires"`
}
type Response struct {
	Visible      *bool           `json:"visible,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
	ID           string          `json:"id"`
	Code         string          `json:"code,omitempty"`
	Error        string          `json:"error,omitempty"`
	Image        []byte          `json:"image,omitempty"`
	Width        int             `json:"width,omitempty"`
	Height       int             `json:"height,omitempty"`
	Display      int             `json:"display"`
	DisplayCount int             `json:"displayCount"`
}

const Prefix = "YOURDESK_AGENT_EVENT "

// CodeRequestInterrupted means execution/result delivery was interrupted or
// temporarily busy. It is not evidence of a permanent source-file failure.
const CodeRequestInterrupted = "request_interrupted"
