//go:build windows

package terminal

import (
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

type winConsole struct {
	read, write      *os.File
	pc, process, job windows.Handle
	mu               sync.Mutex
	closed           bool
	waitDone         chan struct{}
	waitErr          error
}

func supported() bool {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) || user.User.Sid.IsWellKnown(windows.WinLocalServiceSid) || user.User.Sid.IsWellKnown(windows.WinNetworkServiceSid) {
		return false
	}
	var sessionID uint32
	if windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID) != nil || sessionID == 0 {
		return false
	}
	return windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole").Find() == nil
}
func start(cols, rows int) (console, error) {
	var inR, inW, outR, outW windows.Handle
	if e := windows.CreatePipe(&inR, &inW, nil, 0); e != nil {
		return nil, e
	}
	defer windows.CloseHandle(inR)
	if e := windows.CreatePipe(&outR, &outW, nil, 0); e != nil {
		windows.CloseHandle(inW)
		return nil, e
	}
	defer windows.CloseHandle(outW)
	c := &winConsole{waitDone: make(chan struct{}), read: os.NewFile(uintptr(outR), "terminal-output"), write: os.NewFile(uintptr(inW), "terminal-input")}
	ok := false
	defer func() {
		if !ok {
			c.Close()
		}
	}()
	if e := windows.CreatePseudoConsole(windows.Coord{X: int16(cols), Y: int16(rows)}, inR, outW, 0, &c.pc); e != nil {
		return nil, e
	}
	attrs, e := windows.NewProcThreadAttributeList(1)
	if e != nil {
		return nil, e
	}
	defer attrs.Delete()
	if e = attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(c.pc), unsafe.Sizeof(c.pc)); e != nil {
		return nil, e
	}
	si := windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(si))
	// Host 的標準輸入輸出是私有控制管線；不要讓 CMD 沿用父程序的控制管線。
	// ConPTY 負責提供控制台標準 Handle，使用明確的空值覆寫父程序的重新導向。
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = 0, 0, 0
	si.ProcThreadAttributeList = attrs.List()
	dir, e := windows.GetSystemDirectory()
	if e != nil {
		return nil, e
	}
	shell := filepath.Join(dir, "cmd.exe")
	app, _ := windows.UTF16PtrFromString(shell)
	line, _ := windows.UTF16PtrFromString(`"` + shell + `" /D /Q /K prompt $P$G`)
	home, _ := os.UserHomeDir()
	cwd, _ := windows.UTF16PtrFromString(home)
	c.job, e = windows.CreateJobObject(nil, nil)
	if e != nil {
		return nil, e
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, e = windows.SetInformationJobObject(c.job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); e != nil {
		return nil, e
	}
	var pi windows.ProcessInformation
	if e = windows.CreateProcess(app, line, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_SUSPENDED, nil, cwd, &si.StartupInfo, &pi); e != nil {
		return nil, e
	}
	c.process = pi.Process
	go func() {
		_, waitErr := windows.WaitForSingleObject(c.process, windows.INFINITE)
		var code uint32
		if waitErr != nil {
			c.waitErr = waitErr
		} else if err := windows.GetExitCodeProcess(c.process, &code); err != nil {
			c.waitErr = err
		} else if code != 0 {
			c.waitErr = fmt.Errorf("CMD 結束（代碼 %d）", code)
		}
		close(c.waitDone)
	}()
	defer windows.CloseHandle(pi.Thread)
	if e = windows.AssignProcessToJobObject(c.job, c.process); e != nil {
		windows.TerminateProcess(c.process, 1)
		return nil, e
	}
	if _, e = windows.ResumeThread(pi.Thread); e != nil {
		return nil, e
	}
	ok = true
	return c, nil
}
func (c *winConsole) Read(b []byte) (int, error)  { return c.read.Read(b) }
func (c *winConsole) Write(b []byte) (int, error) { return c.write.Write(b) }
func (c *winConsole) Resize(cols, rows int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return os.ErrClosed
	}
	return windows.ResizePseudoConsole(c.pc, windows.Coord{X: int16(cols), Y: int16(rows)})
}
func (c *winConsole) Wait() error {
	<-c.waitDone
	return c.waitErr
}
func (c *winConsole) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.job != 0 {
		windows.CloseHandle(c.job)
	}
	c.write.Close()
	if c.pc != 0 {
		go func() {
			// ClosePseudoConsole 可能等待輸出排空；關閉時由專用讀取者排空。
			go io.Copy(io.Discard, c.read)
			windows.ClosePseudoConsole(c.pc)
			c.read.Close()
			if c.process != 0 {
				<-c.waitDone
				windows.CloseHandle(c.process)
			}
		}()
	} else {
		c.read.Close()
	}
	return nil
}
