package remotedata

import (
	"context"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"time"
	"unsafe"
)

func openRegular(root *os.Root, name string) (*os.File, error) { return checkRegular(root.Open(name)) }

// 先暫停建立程序，加入 Job 後才執行，關閉 Job 時一併結束其子程序。
func runShell(ctx context.Context, in Shell) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	var rd, wr windows.Handle
	if err = windows.CreatePipe(&rd, &wr, &sa, 0); err != nil {
		return nil, err
	}
	reader := os.NewFile(uintptr(rd), "shell-output")
	defer reader.Close()
	defer func() {
		if wr != 0 {
			windows.CloseHandle(wr)
		}
	}()
	if err = windows.SetHandleInformation(rd, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, err
	}
	nul, _ := windows.UTF16PtrFromString("NUL")
	stdin, err := windows.CreateFile(nul, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, &sa, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(stdin)
	sysdir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	shell := filepath.Join(sysdir, "cmd.exe")
	app, _ := windows.UTF16PtrFromString(shell)
	line, err := windows.UTF16PtrFromString(`"` + shell + `" /D /S /C "` + in.Command + `"`)
	if err != nil {
		return nil, err
	}
	dir, err := windows.UTF16PtrFromString(in.Directory)
	if err != nil {
		return nil, err
	}
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attrs.Delete()
	handles := []windows.Handle{stdin, wr}
	if err = attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, err
	}
	si := windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput = stdin
	si.StdOutput = wr
	si.StdErr = wr
	si.ProcThreadAttributeList = attrs.List()
	var pi windows.ProcessInformation
	if err = windows.CreateProcess(app, line, nil, nil, true, windows.CREATE_SUSPENDED|windows.CREATE_NO_WINDOW|windows.EXTENDED_STARTUPINFO_PRESENT, nil, dir, &si.StartupInfo, &pi); err != nil {
		return nil, err
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)
	if err = windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		windows.TerminateProcess(pi.Process, 1)
		return nil, err
	}
	defer windows.TerminateJobObject(job, 1)
	if _, err = windows.ResumeThread(pi.Thread); err != nil {
		return nil, err
	}
	windows.CloseHandle(wr)
	wr = 0
	var out outputBuffer
	done := make(chan struct{})
	go func() { defer close(done); _, _ = io.Copy(&out, reader) }()
	timedOut := false
	for {
		status, e := windows.WaitForSingleObject(pi.Process, 50)
		if e != nil {
			windows.TerminateJobObject(job, 1)
			<-done
			return nil, e
		}
		if status == windows.WAIT_OBJECT_0 {
			break
		}
		if ctx.Err() != nil {
			timedOut = true
			windows.TerminateJobObject(job, 1)
			windows.WaitForSingleObject(pi.Process, 1000)
			break
		}
	}
	var code uint32
	err = windows.GetExitCodeProcess(pi.Process, &code)
	windows.TerminateJobObject(job, 1)
	select {
	case <-done:
	case <-time.After(time.Second):
		reader.Close()
		<-done
	}
	if err != nil {
		return nil, err
	}
	return out.result(int(code), timedOut, shell), nil
}

func openDirectory(root *os.Root, name string) (*os.File, error) { return root.Open(name) }
