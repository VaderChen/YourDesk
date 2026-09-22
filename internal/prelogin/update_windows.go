package prelogin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"yourdesk/internal/buildinfo"
	"yourdesk/internal/serviceupdate"

	winio "github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const updatePipePrefix = `\\.\pipe\YourDeskPrelogin-update-`

type updateReply struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
	Ticket   string `json:"ticket,omitempty"`
	Error    string `json:"error,omitempty"`
}
type updateJob struct {
	URL     string `json:"url"`
	SID     string `json:"sid"`
	Session uint32 `json:"session"`
}

// PrepareServiceUpdate 只有已驗證的舊服務「不認識能力查詢」才回報 legacy。
// 服務離線、GitHub 錯誤、新版服務拒絕請求，一律不能改走 RunAs。
func PrepareServiceUpdate(ctx context.Context, assetURL string) (ticket string, legacy bool, err error) {
	capability, err := updateControl(ctx, "update-capabilities", "")
	if errors.Is(err, io.EOF) {
		root, _, pathErr := servicePaths()
		if pathErr != nil {
			return "", false, pathErr
		}
		marker := filepath.Join(root, "updater.protocol")
		if pathErr = noReparsePath(marker); pathErr != nil {
			return "", false, pathErr
		}
		_, markerErr := os.Stat(marker)
		if os.IsNotExist(markerErr) {
			return "", true, nil
		}
		// 新服務中途退出也可能得到 EOF；已支援自行更新時不可誤判成舊版再次提權。
		return "", false, errors.New("服務更新管線中斷，請稍後重試；不重新要求管理員授權")
	}
	if err != nil {
		return "", false, err
	}
	if capability.Protocol != 1 {
		return "", false, errors.New("不支援的服務更新協定")
	}
	reply, err := updateControl(ctx, "update-prepare", assetURL)
	if err != nil {
		return "", false, err
	}
	if !validUpdateTicket(reply.Ticket) {
		return "", false, errors.New("服務未提供有效的更新工作")
	}
	ticket = reply.Ticket
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	defer func() {
		if err != nil {
			cancelCtx, cancelJob := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelJob()
			_, _ = updateWorkerRequest(cancelCtx, ticket, "rollback")
		}
	}()
	lastResponse := time.Now()
	for {
		var state serviceupdate.Reply
		state, err = updateWorkerRequest(ctx, ticket, "status")
		if err == nil && state.Phase == "ready" {
			return ticket, false, nil
		}
		if err == nil && state.Error != "" {
			err = errors.New(state.Error)
			return ticket, false, err
		}
		if err != nil && time.Since(lastResponse) > 10*time.Second {
			return ticket, false, fmt.Errorf("服務更新程序未回應：%w", err)
		}
		if err == nil {
			lastResponse = time.Now()
			if state.Phase != "preparing" {
				return ticket, false, errors.New("服務更新準備已中止")
			}
		}
		// 管線建立有短暫延遲；準備下載期間不讓 APP 退出。
		select {
		case <-ctx.Done():
			return ticket, false, fmt.Errorf("服務更新準備逾時或取消：%w", ctx.Err())
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func markServiceUpdater(root string) error {
	marker := filepath.Join(root, "updater.protocol")
	if err := noReparsePath(marker); err != nil {
		return err
	}
	return os.WriteFile(marker, []byte("1\n"), 0644)
}

func updateControl(ctx context.Context, operation, assetURL string) (updateReply, error) {
	var reply updateReply
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, controlPipe)
	if err != nil {
		return reply, fmt.Errorf("登入前服務尚未就緒：%w", err)
	}
	defer conn.Close()
	if err = verifyControlServer(conn); err != nil {
		return reply, err
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	if err = json.NewEncoder(conn).Encode(map[string]string{"operation": operation, "url": assetURL}); err != nil {
		return reply, err
	}
	err = json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&reply)
	if err == nil && reply.Error != "" {
		err = errors.New(reply.Error)
	}
	return reply, err
}

func verifyControlServer(conn net.Conn) error {
	pipe, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("無法驗證服務管線")
	}
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(windows.Handle(pipe.Fd()), &pid); err != nil {
		return err
	}
	s, err := openUpdateService(false)
	if err != nil {
		return err
	}
	defer s.Close()
	state, err := s.Query()
	if err != nil {
		return err
	}
	if pid == 0 || pid != state.ProcessId {
		return errors.New("服務管線來源不符")
	}
	return nil
}

func openUpdateService(write bool) (*mgr.Service, error) {
	h, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, err
	}
	defer windows.CloseServiceHandle(h)
	access := uint32(windows.SERVICE_QUERY_STATUS | windows.SERVICE_QUERY_CONFIG)
	if write {
		access |= windows.SERVICE_START | windows.SERVICE_STOP
	}
	name, _ := windows.UTF16PtrFromString(serviceName)
	sh, err := windows.OpenService(h, name, access)
	if err != nil {
		return nil, err
	}
	s := &mgr.Service{Name: serviceName, Handle: sh}
	c, err := s.Config()
	_, exe, pathsErr := servicePaths()
	args, parseErr := windows.DecomposeCommandLine(c.BinaryPathName)
	if err != nil || pathsErr != nil || parseErr != nil || len(args) != 3 || !strings.EqualFold(args[0], exe) || args[1] != "--prelogin" || args[2] != "daemon" || !strings.EqualFold(c.ServiceStartName, "LocalSystem") {
		s.Close()
		return nil, errors.New("登入前服務設定與預期不符")
	}
	return s, nil
}

func validUpdateTicket(id string) bool {
	data, err := hex.DecodeString(id)
	return err == nil && len(id) == 32 && len(data) == 16 && strings.ToLower(id) == id
}

func updateWorkerRequest(ctx context.Context, ticket, operation string) (serviceupdate.Reply, error) {
	var reply serviceupdate.Reply
	if !validUpdateTicket(ticket) {
		return reply, errors.New("無效更新工作")
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, updatePipePrefix+ticket)
	if err != nil {
		return reply, err
	}
	defer conn.Close()
	d, _ := ctx.Deadline()
	_ = conn.SetDeadline(d)
	if err = json.NewEncoder(conn).Encode(map[string]string{"operation": operation}); err != nil {
		return reply, err
	}
	err = json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&reply)
	return reply, err
}

func CancelServiceUpdate(ctx context.Context, ticket string) {
	_, _ = updateWorkerRequest(ctx, ticket, "rollback")
}

var updateLaunch sync.Mutex

func startServiceUpdate(conn net.Conn, assetURL string) (string, error) {
	if err := requireServiceProcess(); err != nil {
		return "", err
	}
	if _, _, err := serviceupdate.Identity(assetURL, runtime.GOARCH); err != nil {
		return "", err
	}
	identity, err := updateClient(conn)
	if err != nil {
		return "", err
	}
	if identity.Session == 0 || identity.Session != windows.WTSGetActiveConsoleSessionId() {
		return "", errors.New("請在目前主控台更新 APP")
	}
	if !updateLaunch.TryLock() {
		return "", errors.New("已有服務更新正在進行")
	}
	unlock := true
	defer func() {
		if unlock {
			updateLaunch.Unlock()
		}
	}()
	root, _, err := servicePaths()
	if err != nil {
		return "", err
	}
	lock, err := acquireUpdateLock(root)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	jobs := filepath.Join(root, "updates")
	if err = os.MkdirAll(jobs, 0700); err != nil {
		return "", err
	}
	if err = noReparsePath(jobs); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(jobs)
	if err != nil {
		return "", err
	}
	remaining := 0
	for _, entry := range entries {
		if !validUpdateTicket(entry.Name()) {
			continue
		}
		previous := filepath.Join(jobs, entry.Name())
		if _, e := os.Stat(filepath.Join(previous, "complete")); e == nil {
			if noReparsePath(previous) == nil {
				_ = os.RemoveAll(previous)
			}
		} else {
			remaining++
		}
	}
	if remaining >= 3 {
		return "", errors.New("先前服務更新備份尚待檢查，請由管理員查看 YourDeskPrelogin/updates")
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(id)
	dir := filepath.Join(jobs, ticket)
	if err = os.Mkdir(dir, 0700); err != nil {
		return "", err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()
	for _, name := range serviceupdate.Files {
		if err = copyServiceFile(root, dir, name); err != nil {
			return "", err
		}
	}
	identity.URL = assetURL
	data, _ := json.Marshal(identity)
	if err = secureWrite(filepath.Join(dir, "job.json"), data, "D:P(A;;FA;;;SY)(A;;FA;;;BA)"); err != nil {
		return "", err
	}
	logFile, err := os.OpenFile(filepath.Join(dir, "update.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer logFile.Close()
	cmd := exec.Command(filepath.Join(dir, "yourdesk-client.exe"), "--prelogin", "update-worker", ticket)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP}
	if err = cmd.Start(); err != nil {
		return "", err
	}
	// 不放入桌面代理的 Job Object；停止 SCM 服務時 worker 必須繼續完成或回復。
	keep, unlock = true, false
	go func() { _ = cmd.Wait(); updateLaunch.Unlock() }()
	return ticket, nil
}

func updateClient(conn net.Conn) (updateJob, error) {
	var result updateJob
	pipe, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return result, errors.New("無法驗證更新來源")
	}
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(windows.Handle(pipe.Fd()), &pid); err != nil {
		return result, err
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return result, err
	}
	defer windows.CloseHandle(proc)
	var token windows.Token
	if err = windows.OpenProcessToken(proc, windows.TOKEN_QUERY, &token); err != nil {
		return result, err
	}
	defer token.Close()
	u, err := token.GetTokenUser()
	if err != nil {
		return result, err
	}
	result.SID = u.User.Sid.String()
	if err = windows.ProcessIdToSessionId(pid, &result.Session); err != nil {
		return result, err
	}
	return result, nil
}

func noReparsePath(path string) error {
	for {
		p, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(p)
		if err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) && !errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return err
		}
		if err == nil && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("拒絕服務更新路徑中的連結")
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

func acquireUpdateLock(root string) (*os.File, error) {
	name := filepath.Join(root, "update.lock")
	if err := noReparsePath(name); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{}); err != nil {
		f.Close()
		return nil, errors.New("已有服務更新工作持有鎖定")
	}
	return f, nil
}

func copyServiceFile(source, destination, name string) error {
	from, to := filepath.Join(source, name), filepath.Join(destination, name)
	if err := noReparsePath(from); err != nil {
		return err
	}
	if err := noReparsePath(to); err != nil {
		return err
	}
	input, err := os.Open(from)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("服務來源不是一般檔案")
	}
	output, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	closeErr := output.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}

func runUpdateWorker(ctx context.Context, ticket string) error {
	if !validUpdateTicket(ticket) {
		return errors.New("無效的更新工作")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || !user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		return errors.New("服務更新只能由 SYSTEM 執行")
	}
	root, _, err := servicePaths()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "updates", ticket)
	self, err := os.Executable()
	if err != nil || !strings.EqualFold(self, filepath.Join(dir, "yourdesk-client.exe")) {
		return errors.New("服務更新程序位置不符")
	}
	if err = noReparsePath(dir); err != nil {
		return err
	}
	// 等待啟動者交出鎖；服務重啟後仍以此鎖排除第二個更新工作。
	var lock *os.File
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		lock, err = acquireUpdateLock(root)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	data, err := os.ReadFile(filepath.Join(dir, "job.json"))
	if err != nil {
		return err
	}
	var job updateJob
	if json.Unmarshal(data, &job) != nil || len(data) > 4096 {
		return errors.New("無效的服務更新設定")
	}
	if _, err = windows.StringToSid(job.SID); err != nil {
		return err
	}
	listener, err := winio.ListenPipe(updatePipePrefix+ticket, &winio.PipeConfig{SecurityDescriptor: "D:P(D;;GA;;;NU)(A;;GA;;;SY)(A;;GRGW;;;" + job.SID + ")"})
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	var mu sync.Mutex
	state := serviceupdate.Reply{Phase: "preparing"}
	var transaction *serviceupdate.Transaction
	done := make(chan struct{})
	var finish sync.Once
	go func() {
		version, prepareErr := serviceupdate.Prepare(ctx, job.URL, runtime.GOARCH, buildinfo.Current(), dir)
		mu.Lock()
		defer mu.Unlock()
		if state.Phase != "preparing" {
			return
		}
		if prepareErr != nil {
			state = serviceupdate.Reply{Phase: "failed", Error: prepareErr.Error()}
			return
		}
		transaction = serviceupdate.NewTransaction(&windowsUpdateOperations{root: root, dir: dir, version: version, previous: buildinfo.Current()})
		state.Phase = "ready"
	}()
	go func() {
		slots := make(chan struct{}, 4)
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			select {
			case slots <- struct{}{}:
			default:
				conn.Close()
				continue
			}
			go func() {
				defer func() { conn.Close(); <-slots }()
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				caller, e := updateClient(conn)
				if e != nil || caller.SID != job.SID || caller.Session != job.Session {
					return
				}
				var request struct {
					Operation string `json:"operation"`
				}
				if json.NewDecoder(io.LimitReader(conn, 1024)).Decode(&request) != nil {
					return
				}
				_ = conn.SetDeadline(time.Now().Add(3 * time.Minute))
				mu.Lock()
				defer mu.Unlock()
				reply := state
				if transaction != nil {
					reply = transaction.Command(request.Operation)
					state = reply
				} else if request.Operation == "rollback" {
					cancel()
					state = serviceupdate.Reply{Phase: "rolled-back"}
					reply = state
				} else if request.Operation != "status" {
					reply.Error = "服務套件尚未準備完成"
				}
				_ = json.NewEncoder(conn).Encode(reply)
				if reply.Phase == "complete" || reply.Phase == "rolled-back" {
					// 留短暫重送時間，避免確認回覆遺失時把成功更新誤報為失敗。
					finish.Do(func() { go func() { time.Sleep(5 * time.Second); close(done) }() })
				}
			}()
		}
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	cancel()
	mu.Lock()
	defer mu.Unlock()
	if transaction != nil && transaction.Phase != "complete" {
		if err = transaction.Rollback(); err != nil {
			return fmt.Errorf("服務更新回復失敗，備份保留於 %s：%w", dir, err)
		}
	}
	// 執行中的 worker 檔案不能刪除；下次更新在取得排他鎖後清除完成工作。
	return os.WriteFile(filepath.Join(dir, "complete"), []byte("complete"), 0600)
}

type windowsUpdateOperations struct{ root, dir, version, previous string }

func (o *windowsUpdateOperations) Stop() error {
	s, err := openUpdateService(true)
	if err != nil {
		return err
	}
	defer s.Close()
	status, err := s.Query()
	if err != nil {
		return err
	}
	var process windows.Handle
	if status.ProcessId != 0 {
		process, err = windows.OpenProcess(windows.SYNCHRONIZE, false, status.ProcessId)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(process)
	}
	if status.State != svc.StopPending && status.State != svc.Stopped {
		_, err = s.Control(svc.Stop)
	}
	if err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return err
	}
	if err = waitService(s, svc.Stopped, 30*time.Second); err != nil {
		return err
	}
	// SCM 停止狀態與程序卸載 DLL 之間可能有時間差，先等程序真正退出。
	if process != 0 {
		result, err := windows.WaitForSingleObject(process, 30000)
		if err != nil {
			return err
		}
		if result != windows.WAIT_OBJECT_0 {
			return errors.New("服務程序尚未完全退出")
		}
	}
	return nil
}
func (o *windowsUpdateOperations) Start() error {
	s, err := openUpdateService(true)
	if err != nil {
		return err
	}
	defer s.Close()
	if err = s.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return err
	}
	return waitService(s, svc.Running, 30*time.Second)
}
func (o *windowsUpdateOperations) Backup() error {
	dir := filepath.Join(o.dir, "previous")
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	for _, name := range serviceupdate.Files {
		if err := copyServiceFile(o.root, dir, name); err != nil {
			return err
		}
	}
	return nil
}
func (o *windowsUpdateOperations) Install() error {
	for _, name := range serviceupdate.Files {
		if err := copyServiceFile(filepath.Join(o.dir, "next"), o.root, name); err != nil {
			return err
		}
	}
	return nil
}
func (o *windowsUpdateOperations) Restore() error {
	for _, name := range serviceupdate.Files {
		if err := copyServiceFile(filepath.Join(o.dir, "previous"), o.root, name); err != nil {
			return err
		}
	}
	return nil
}
func (o *windowsUpdateOperations) Verify(updated bool) error {
	want := o.previous
	if updated {
		want = o.version
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		r, err := updateControl(ctx, "update-capabilities", "")
		if err == nil && r.Protocol == 1 && serviceupdate.VersionKey(r.Version) == serviceupdate.VersionKey(want) {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("更新後的服務版本或控制管線未通過健康檢查")
}
