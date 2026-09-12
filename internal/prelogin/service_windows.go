package prelogin

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const serviceName = "YourDeskPrelogin"
const serviceHint = "開機後自動啟動，登入前即可接受連線；啟用與停用需要管理員授權。"
const serviceFailed = "無法變更登入前啟動，請確認管理員授權後重試。"

func servicePaths() (root, executable string, err error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return "", "", err
	}
	root = filepath.Join(base, serviceName)
	return root, filepath.Join(root, "yourdesk-client.exe"), nil
}

func Status() State {
	s := State{Supported: true, Message: serviceHint}
	// WinPE 不安裝持久服務。
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\MiniNT`, registry.QUERY_VALUE); err == nil {
		key.Close()
		return State{Message: "這台電腦目前不支援登入前連線。"}
	}
	root, _, err := servicePaths()
	if err != nil {
		return State{Message: serviceFailed}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		// 保留目錄表示服務存在或尚待清理，避免 APP 啟動第二份 Host。
		s.Enabled = true
		var public struct {
			Room string `json:"room"`
		}
		if data, err := os.ReadFile(filepath.Join(root, "public.json")); err == nil {
			if json.Unmarshal(data, &public) == nil {
				s.Room = public.Room
			}
		}
	}
	return s
}

func Configure(ctx context.Context, enabled bool, c Config) error {
	if !Status().Supported {
		return errors.New(Status().Message)
	}
	mode, path := "remove", ""
	if enabled {
		if err := c.validate(); err != nil {
			return err
		}
		stage, err := os.MkdirTemp("", "yourdesk-prelogin-")
		if err != nil {
			return errors.New(serviceFailed)
		}
		defer os.RemoveAll(stage)
		path = filepath.Join(stage, "config.json")
		data, err := json.Marshal(c)
		if err != nil {
			return errors.New(serviceFailed)
		}
		// 暫存密碼僅供目前使用者、SYSTEM 與管理員讀取。
		user, err := windows.GetCurrentProcessToken().GetTokenUser()
		if err != nil {
			return errors.New(serviceFailed)
		}
		sddl := "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + user.User.Sid.String() + ")"
		if err = secureWrite(path, data, sddl); err != nil {
			return errors.New(serviceFailed)
		}
		mode = "install"
	}
	if err := elevateWindows(ctx, mode, path); err != nil {
		slog.Warn("登入前服務操作未完成", "operation", mode, "error", err)
		return errors.New(serviceFailed)
	}
	return nil
}

func elevateWindows(ctx context.Context, mode, path string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	args := "--prelogin " + mode
	if path != "" {
		args += " " + syscall.EscapeArg(path)
	}
	script := "$ErrorActionPreference='Stop'; $p=Start-Process -FilePath " + quote(self) + " -ArgumentList " + quote(args) + " -Verb RunAs -Wait -PassThru; exit $p.ExitCode"
	encoded := utf16.Encode([]rune(script))
	data := make([]byte, len(encoded)*2)
	for i, v := range encoded {
		binary.LittleEndian.PutUint16(data[i*2:], v)
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(system, `WindowsPowerShell\v1.0\powershell.exe`), "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd.Run()
}

// 安裝時原子建立 ACL；Windows 不以 Unix 的 0600 位元保護密碼。
func secureWrite(path string, data []byte, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(h), path)
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func install(path string) error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New(serviceFailed)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, 64<<10))
	f.Close()
	if err != nil {
		return err
	}
	var c Config
	if err = json.Unmarshal(data, &c); err != nil {
		return err
	}
	if err = c.validate(); err != nil {
		return err
	}
	root, executable, err := servicePaths()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if existing, e := m.OpenService(serviceName); e == nil {
		existing.Close()
		return errors.New("登入前服務已存在，請先停用後再啟用。")
	} else if !errors.Is(e, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return e
	}
	// 受保護的獨立副本，不讓 SYSTEM 執行使用者可覆寫的安裝位置。
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;AU)")
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	name, _ := windows.UTF16PtrFromString(root)
	if err = windows.CreateDirectory(name, &sa); err != nil {
		return err
	}
	// 僅清理本次成功建立的目錄，既有路徑一律不覆寫。
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(root)
		}
	}()
	self, err := os.Executable()
	if err != nil {
		return err
	}
	binaryData, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	if err = os.WriteFile(executable, binaryData, 0600); err != nil {
		return err
	}
	if err = secureWrite(filepath.Join(root, "config.json"), data, "D:P(A;;FA;;;SY)(A;;FA;;;BA)"); err != nil {
		return err
	}
	if err = secureWrite(filepath.Join(root, "host.lock"), nil, "D:P(A;;FA;;;SY)(A;;FA;;;BA)"); err != nil {
		return err
	}
	public, _ := json.Marshal(map[string]string{"room": c.Room})
	if err = os.WriteFile(filepath.Join(root, "public.json"), public, 0644); err != nil {
		return err
	}
	s, err := m.CreateService(serviceName, executable, mgr.Config{StartType: mgr.StartAutomatic, ErrorControl: mgr.ErrorNormal, ServiceStartName: "LocalSystem", DisplayName: "YourDesk 登入前啟動", Description: "開機後自動啟動 YourDesk，提供登入前與登入後的遠端連線。"}, "--prelogin", "daemon")
	if err != nil {
		return err
	}
	defer s.Close()
	// SCM 已指向此副本，後續失敗保留目錄供停用流程清理。
	keep = true
	if err = s.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}}, 86400); err != nil {
		return err
	}
	if err = s.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return err
	}
	if err = s.Start(); err != nil {
		return err
	}
	return waitService(s, svc.Running, 20*time.Second)
}

func waitService(s *mgr.Service, target svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := s.Query()
		if err != nil {
			return err
		}
		if status.State == target {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("服務尚未完成狀態切換")
}

func remove() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New(serviceFailed)
	}
	root, _, err := servicePaths()
	if err != nil {
		return err
	}
	// 不追蹤可疑目錄連結，也不刪除服務範圍以外的資料。
	if info, e := os.Lstat(root); e == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return errors.New(serviceFailed)
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err == nil {
		defer s.Close()
		if _, err = s.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			return err
		}
		if err = waitService(s, svc.Stopped, 30*time.Second); err != nil {
			return err
		}
		if err = s.Delete(); err != nil {
			return err
		}
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	return os.RemoveAll(root)
}

func AcquireHost(ctx context.Context) (func(), error) {
	if err := requireServiceProcess(); err != nil {
		return nil, err
	}
	root, _, err := servicePaths()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(root, "host.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{}); err == nil {
			return func() { f.Close() }, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func requireServiceProcess() error {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	_, executable, err := servicePaths()
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if !u.User.Sid.IsWellKnown(windows.WinLocalSystemSid) || !strings.EqualFold(self, executable) {
		return errors.New("服務程序只能由系統啟動。")
	}
	return nil
}
