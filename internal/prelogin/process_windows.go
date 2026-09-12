package prelogin

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	winio "github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"
)

// 私有管線傳遞配對資料；Job Object 確保服務或代理退出時回收整棵子程序。
type serviceChild struct {
	input        io.WriteCloser
	output       io.ReadCloser
	process, job windows.Handle
	done         chan struct{}
	mu           sync.Mutex
}

func (c *serviceChild) send(value any) error {
	done := make(chan error, 1)
	go func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		done <- json.NewEncoder(c.input).Encode(value)
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		c.input.Close()
		return errors.New("服務控制管線逾時")
	}
}
func (c *serviceChild) stop() {
	c.input.Close()
	select {
	case <-c.done:
	case <-time.After(3 * time.Second):
		_ = windows.TerminateJobObject(c.job, 1)
		<-c.done
	}
	c.output.Close()
	windows.CloseHandle(c.process)
	windows.CloseHandle(c.job)
	c.job = 0
}

func enableServicePrivilege(name string) error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	value, _ := windows.UTF16PtrFromString(name)
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, value, &luid); err != nil {
		return err
	}
	p := windows.Tokenprivileges{PrivilegeCount: 1}
	p.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	return windows.AdjustTokenPrivileges(token, false, &p, 0, nil, nil)
}

func startServiceChild(session uint32, desktop string, args []string) (*serviceChild, error) {
	if session == 0 || session == 0xffffffff {
		return nil, errors.New("尚無可用的主控台工作階段")
	}
	if err := requireServiceProcess(); err != nil {
		return nil, err
	}
	var currentSession uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &currentSession); err != nil {
		return nil, err
	}
	// Windows 不允許跨工作階段繼承 Handle；服務到代理使用 SYSTEM 專用管線。
	var listener net.Listener
	if currentSession != session {
		if len(args) != 2 || args[0] != "--prelogin" || args[1] != "agent" {
			return nil, errors.New("跨工作階段只能啟動服務代理")
		}
		name := fmt.Sprintf(`\\.\pipe\YourDeskPrelogin-%x`, rand.Text())
		var err error
		listener, err = winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: "D:P(D;;GA;;;NU)(A;;GA;;;SY)"})
		if err != nil {
			return nil, err
		}
		defer listener.Close()
		args = append(args, name)
	}
	if err := enableServicePrivilege("SeTcbPrivilege"); err != nil {
		return nil, err
	}
	// 複製主權杖需要 TOKEN_DUPLICATE，不能使用僅供查詢的偽 Handle。
	var sourceToken windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY, &sourceToken); err != nil {
		return nil, err
	}
	defer sourceToken.Close()
	var token windows.Token
	if err := windows.DuplicateTokenEx(sourceToken, windows.TOKEN_ALL_ACCESS, nil, windows.SecurityImpersonation, windows.TokenPrimary, &token); err != nil {
		return nil, err
	}
	defer token.Close()
	if err := windows.SetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4); err != nil {
		return nil, err
	}
	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	if err := windows.CreatePipe(&inputRead, &inputWrite, &sa, 0); err != nil {
		return nil, err
	}
	defer windows.CloseHandle(inputRead)
	if err := windows.CreatePipe(&outputRead, &outputWrite, &sa, 0); err != nil {
		windows.CloseHandle(inputWrite)
		return nil, err
	}
	defer windows.CloseHandle(outputWrite)
	c := &serviceChild{input: os.NewFile(uintptr(inputWrite), "service-input"), output: os.NewFile(uintptr(outputRead), "service-output"), done: make(chan struct{})}
	ok := false
	defer func() {
		if !ok {
			c.input.Close()
			c.output.Close()
			if c.job != 0 {
				windows.CloseHandle(c.job)
			}
		}
	}()
	if err := windows.SetHandleInformation(inputWrite, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, err
	}
	if err := windows.SetHandleInformation(outputRead, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, err
	}
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	handles := []windows.Handle{inputRead, outputWrite}
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, err
	}
	si := windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = inputRead, outputWrite, outputWrite
	si.Desktop, err = windows.UTF16PtrFromString(`winsta0\` + desktop)
	if err != nil {
		return nil, err
	}
	si.ProcThreadAttributeList = attributes.List()
	inherit := listener == nil
	if !inherit {
		si.Flags = 0
		si.StdInput, si.StdOutput, si.StdErr = 0, 0, 0
		si.ProcThreadAttributeList = nil
		si.Cb = uint32(unsafe.Sizeof(windows.StartupInfo{}))
	}
	root, executable, err := servicePaths()
	if err != nil {
		return nil, err
	}
	line := []string{executable}
	line = append(line, args...)
	for i := range line {
		line[i] = syscall.EscapeArg(line[i])
	}
	command, err := windows.UTF16PtrFromString(strings.Join(line, " "))
	if err != nil {
		return nil, err
	}
	application, _ := windows.UTF16PtrFromString(executable)
	directory, _ := windows.UTF16PtrFromString(root)
	var environment *uint16
	if err = windows.CreateEnvironmentBlock(&environment, token, false); err != nil {
		return nil, err
	}
	defer windows.DestroyEnvironmentBlock(environment)
	c.job, err = windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(c.job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, err
	}
	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW | windows.CREATE_UNICODE_ENVIRONMENT | windows.EXTENDED_STARTUPINFO_PRESENT)
	if !inherit {
		flags &^= windows.EXTENDED_STARTUPINFO_PRESENT
	}
	if err = windows.CreateProcessAsUser(token, application, command, nil, nil, inherit, flags, environment, directory, &si.StartupInfo, &pi); err != nil {
		return nil, err
	}
	defer windows.CloseHandle(pi.Thread)
	if err = windows.AssignProcessToJobObject(c.job, pi.Process); err == nil {
		_, err = windows.ResumeThread(pi.Thread)
	}
	if err != nil {
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Process)
		return nil, err
	}
	c.process = pi.Process
	go func() { windows.WaitForSingleObject(c.process, windows.INFINITE); close(c.done) }()
	if listener != nil {
		accepted := make(chan net.Conn)
		acceptDone := make(chan struct{})
		defer close(acceptDone)
		go func() {
			conn, _ := listener.Accept()
			select {
			case accepted <- conn:
			case <-acceptDone:
				if conn != nil {
					conn.Close()
				}
			}
		}()
		var conn net.Conn
		select {
		case conn = <-accepted:
		case <-c.done:
		case <-time.After(10 * time.Second):
		}
		if conn == nil {
			c.stop()
			return nil, errors.New("桌面代理未能啟動")
		}
		c.input.Close()
		c.output.Close()
		c.input, c.output = conn, conn
	}
	ok = true
	return c, nil
}

type serviceEvent struct {
	Event   string `json:"event"`
	Session uint64 `json:"session"`
}

func watchServiceOutput(ctx context.Context, c *serviceChild, events chan<- serviceEvent) {
	scanner := bufio.NewScanner(c.output)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		payload, ok := strings.CutPrefix(scanner.Text(), "YOURDESK_UI_EVENT ")
		if !ok {
			continue
		}
		var event serviceEvent
		if json.Unmarshal([]byte(payload), &event) != nil {
			continue
		}
		if event.Event != "host-connected" && event.Event != "host-disconnected" {
			continue
		}
		select {
		case events <- event:
		case <-ctx.Done():
			return
		}
	}
}
