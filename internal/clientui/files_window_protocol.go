package clientui

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

type filesWindowMessage struct {
	Event string `json:"event,omitempty"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func decodeFilesWindowMessage(data []byte, initial bool) (filesWindowMessage, error) {
	var message filesWindowMessage
	if len(data) == 0 || len(data) > 16384 || !utf8.Valid(data) {
		return message, errors.New("檔案視窗管線訊息無效或過長")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&message); err != nil {
		return message, err
	}
	if decoder.Decode(new(any)) != io.EOF || len(message.Title) > 1024 || (initial && message.Event != "") || (!initial && message.Event != "files-session") {
		return message, errors.New("檔案視窗管線訊息無效")
	}
	if _, _, err := parseFilesAddress(message.URL); err != nil {
		return message, err
	}
	return message, nil
}
