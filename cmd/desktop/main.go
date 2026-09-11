// YourDesk 發行版入口：從執行檔所在目錄啟動原生 Client UI。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"yourdesk/internal/childprocess"
)

func main() {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	name := "yourdesk-client"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	args := append([]string{"-ui", "-signal", "wss://desktop.mars-cloud.com:8080/ws"}, os.Args[1:]...)
	child := childprocess.Command(filepath.Join(filepath.Dir(executable), name), args...)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err = child.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
