package autostart

import (
	"bytes"
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
)

func agentPath() (string, error) {
	home, err := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.yourdesk.autostart.plist"), err
}
func supported(desktop bool) bool { _, err := os.UserHomeDir(); return err == nil && os.Geteuid() != 0 }
func enabled(desktop bool) (bool, error) {
	path, err := agentPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
func configure(ctx context.Context, on bool, c Config) error {
	path, err := agentPath()
	if err != nil {
		return err
	}
	if !on {
		return removeFile(path)
	}
	args := append([]string{c.Executable}, c.Args...)
	if c.Desktop {
		// 由 LaunchServices 開啟 APP，保留應用程式身分與既有單一實例機制。
		for dir := filepath.Dir(c.Executable); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if filepath.Ext(dir) == ".app" {
				if filepath.Base(dir) != "YourDesk.app" {
					return os.ErrInvalid
				}
				args = []string{"/usr/bin/open", "-a", dir, "--args"}
				args = append(args, c.Args...)
				break
			}
		}
	}
	var out bytes.Buffer
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>com.yourdesk.autostart</string><key>ProgramArguments</key><array>`)
	for _, arg := range args {
		out.WriteString("<string>")
		xml.EscapeText(&out, []byte(arg))
		out.WriteString("</string>")
	}
	out.WriteString(`</array><key>RunAtLoad</key><true/><key>LimitLoadToSessionType</key><string>Aqua</string></dict></plist>`)
	// 不 bootstrap：切換開關不另外啟動一份 APP，下次登入才生效。
	return writeFile(path, out.Bytes())
}
