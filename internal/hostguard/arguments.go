package hostguard

import "strings"

// 對未知或介面／查詢模式採保守排除，不以檔名相同就認定為 Host。
func hostArguments(args []string, localRoom string) (room, signal string, host bool) {
	room, signal, host = localRoom, "wss://127.0.0.1:8080/ws", true
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(args[i], "=")
		name = strings.TrimPrefix(name, "-")
		name = strings.TrimPrefix(name, "-")
		switch name {
		case "ui", "print-secret", "print-uid", "check-signal", "h", "help":
			return "", "", false
		case "room", "signal":
			if !hasValue {
				i++
				if i >= len(args) {
					return "", "", false
				}
				value = args[i]
			}
			if name == "room" {
				room = value
				if room == "" {
					room = localRoom
				}
			} else {
				signal = value
			}
		}
	}
	return
}
