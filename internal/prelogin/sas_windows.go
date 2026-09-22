package prelogin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"time"
	"unsafe"

	winio "github.com/tailscale/go-winio"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// 僅服務安裝目錄中的 SYSTEM Host 可請求 SAS；一般 UI 不會取得此能力。
func SecureAttentionAvailable() bool { return requireServiceProcess() == nil }
func SendSecureAttention(ctx context.Context) error {
	if !SecureAttentionAvailable() {
		return errors.New("請先在遠端 Windows 啟用未登入開機服務，再重新連線。")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, controlPipe)
	if err != nil {
		return errors.New("遠端 Windows 登入前服務尚未就緒。")
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	conn.SetDeadline(deadline)
	if err = json.NewEncoder(conn).Encode(map[string]string{"operation": "sas"}); err != nil {
		return err
	}
	var reply struct {
		Error string `json:"error"`
	}
	if err = json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&reply); err != nil {
		return err
	}
	if reply.Error != "" {
		return errors.New(reply.Error)
	}
	return nil
}

// 在真正的 SCM 服務內呼叫；僅接受目前主控台的 SYSTEM Host，不接受客戶端指定 session。
func sendClientSAS(conn net.Conn) error {
	sasToggleMu.RLock()
	defer sasToggleMu.RUnlock()
	if secureAttentionDisabled() {
		return errors.New("遠端 YourDesk 已關閉 Ctrl+Alt+Del，請先在遠端設定開啟。")
	}
	pipe, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("無法驗證 SAS 控制管線")
	}
	var pid uint32
	if err := windows.GetNamedPipeClientProcessId(windows.Handle(pipe.Fd()), &pid); err != nil {
		return err
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(proc)
	var token windows.Token
	if err = windows.OpenProcessToken(proc, windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE, &token); err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	if !user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		return errors.New("SAS 僅接受系統 Host 請求")
	}
	_, expected, err := servicePaths()
	if err != nil {
		return err
	}
	var path [32768]uint16
	n := uint32(len(path))
	if err = windows.QueryFullProcessImageName(proc, 0, &path[0], &n); err != nil {
		return err
	}
	if !strings.EqualFold(windows.UTF16ToString(path[:n]), expected) {
		return errors.New("SAS 請求來源不是已安裝的 Host")
	}
	var session, needed uint32
	if err = windows.GetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4, &needed); err != nil {
		return err
	}
	if session == 0 || session == 0xffffffff || session != windows.WTSGetActiveConsoleSessionId() {
		return errors.New("SAS 請求不屬於目前主控台工作階段")
	}
	// 遵循系統管理原則，不從一般遠端命令更改機器原則。
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	policy, _, err := key.GetIntegerValue("SoftwareSASGeneration")
	if err != nil || (policy != 1 && policy != 3) {
		return errors.New("請在遠端 Windows 原則「停用或啟用軟體安全注意序列」允許服務產生 SAS。")
	}
	send := windows.NewLazySystemDLL("sas.dll").NewProc("SendSAS")
	if err = send.Find(); err != nil {
		return err
	}
	// 目前只支援 active console。以 SCM 服務身分呼叫 SendSAS(FALSE)，
	// 不模擬 Host：其複製的 SYSTEM 權杖會讓 Windows 10 靜默忽略 SAS。
	// 驗證後若主控台已切換，不把指令送到另一個使用者的工作階段。
	if session != windows.WTSGetActiveConsoleSessionId() {
		return errors.New("主控台工作階段已切換，請重新連線後再試。")
	}
	send.Call(0) // VOID API：只回報已提交，不宣稱安全桌面已呈現。
	return nil
}

const sasPolicyKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`

func secureAttentionPolicyAllowed() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, sasPolicyKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("SoftwareSASGeneration")
	return err == nil && (value == 1 || value == 3)
}

// 僅由本機使用者確認後的 UAC 輔助程序呼叫，不由遠端指令修改原則。
func enableSecureAttentionPolicy() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("SAS 授權需要管理員權限")
	}
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, sasPolicyKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("SoftwareSASGeneration")
	if err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return err
	}
	next, err := sasPolicyWithServices(value)
	if err != nil {
		return err
	}
	return key.SetDWordValue("SoftwareSASGeneration", next)
}
