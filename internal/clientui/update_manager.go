package clientui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const updateInterval = 12 * time.Hour
const maxUpdateSize int64 = 2 << 30

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}
type updateStatus struct {
	InstallAt       int64        `json:"installAt"`
	Available       bool         `json:"available"`
	Version         string       `json:"version"`
	Message         string       `json:"message"`
	CheckedAt       time.Time    `json:"checkedAt"`
	NotifiedVersion string       `json:"notifiedVersion"`
	Asset           releaseAsset `json:"asset"`
	Downloading     bool         `json:"downloading"`
	DownloadBytes   int64        `json:"downloadBytes"`
	DownloadPath    string       `json:"downloadPath"`
	DownloadError   string       `json:"downloadError"`
	OpenError       string       `json:"openError"`
	Opening         bool         `json:"opening"`
}
type updateManager struct {
	decision    chan bool
	downloads   sync.WaitGroup
	mu          sync.Mutex
	checkMu     sync.Mutex
	state       updateStatus
	path        string
	ctx         context.Context
	notify      chan struct{}
	forceUpdate bool
}

func newUpdateManager(ctx context.Context, dir string) *updateManager {
	u := &updateManager{ctx: ctx, path: filepath.Join(dir, "updates.json"), notify: make(chan struct{}, 1)}
	// 本機啟動器每次從下載前開始，且不讀寫正式更新紀錄。
	u.forceUpdate = os.Getenv("YOURDESK_TEST_UPDATE") == "1"
	if u.forceUpdate {
		return u
	}
	if data, err := os.ReadFile(u.path); err == nil {
		if err = json.Unmarshal(data, &u.state); err != nil {
			slog.Warn("更新設定讀取失敗", "error", err)
			u.state = updateStatus{}
		}
	}
	u.state.Downloading = false
	u.state.InstallAt = 0
	u.state.Opening = false
	if u.state.DownloadPath == "" {
		u.state.DownloadBytes = 0
	} else {
		u.state.DownloadBytes = u.state.Asset.Size
	}
	// 安裝新版本後，不再沿用舊版本的可更新狀態。
	u.state.Available = versionKey(u.state.Version) != "" && versionKey(currentVersion()) != "" && versionKey(u.state.Version) > versionKey(currentVersion())
	return u
}
func (u *updateManager) snapshot() updateStatus { u.mu.Lock(); defer u.mu.Unlock(); return u.state }
func (u *updateManager) saveLocked() {
	if u.forceUpdate {
		return
	}
	if err := saveJSON(u.path, u.state); err != nil {
		slog.Warn("更新狀態保存失敗", "error", err)
	}
}
func (u *updateManager) notifyPending() {
	state := u.snapshot()
	if state.Available && state.Version != state.NotifiedVersion {
		select {
		case u.notify <- struct{}{}:
		default:
		}
	}
}
func (u *updateManager) run() {
	// 測試模式只由手動檢查觸發，啟動與背景排程不顯示更新提示。
	if u.forceUpdate {
		<-u.ctx.Done()
		return
	}
	u.notifyPending()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		state := u.snapshot()
		if state.CheckedAt.IsZero() || time.Since(state.CheckedAt) >= updateInterval || state.CheckedAt.After(time.Now()) {
			u.check(u.ctx, false)
			u.notifyPending()
		}
		select {
		case <-u.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (u *updateManager) check(ctx context.Context, manual bool, force ...bool) updateStatus {
	u.checkMu.Lock()
	defer u.checkMu.Unlock()
	if state := u.snapshot(); state.Downloading || state.Opening || state.InstallAt > 0 {
		return state
	}
	result, err := fetchRelease(ctx)
	if err == nil && manual && (u.forceUpdate || (len(force) > 0 && force[0])) && validReleaseAsset(result.Asset) {
		result.Available = true
		result.Message = "更新流程測試：可下載目前的正式版本。"
	}
	u.mu.Lock()
	if u.state.Downloading || u.state.Opening || u.state.InstallAt > 0 {
		state := u.state
		u.mu.Unlock()
		return state
	}
	u.state.CheckedAt = time.Now()
	if err != nil {
		u.state.Message = err.Error()
	} else {
		// 版本或資產變更時，不沿用上一版的下載狀態。
		if u.state.Version != result.Version || u.state.Asset != result.Asset {
			u.state.DownloadPath, u.state.DownloadError, u.state.OpenError = "", "", ""
			u.state.DownloadBytes = 0
		}
		u.state.Available, u.state.Version, u.state.Asset, u.state.Message = result.Available, result.Version, result.Asset, result.Message
	}
	u.saveLocked()
	state := u.state
	u.mu.Unlock()
	return state
}
func fetchRelease(ctx context.Context) (updateStatus, error) {
	var result updateStatus
	request, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/VaderChen/YourDesk/releases/latest", nil)
	if err != nil {
		return result, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "YourDesk")
	response, err := (&http.Client{Timeout: 8 * time.Second}).Do(request)
	if err != nil {
		return result, fmt.Errorf("無法連線 GitHub，請稍後重試。")
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		result.Message = "目前沒有可取得的正式發行版本。"
		return result, nil
	}
	if response.StatusCode != 200 {
		return result, fmt.Errorf("GitHub 暫時無法提供更新資訊，請稍後重試。")
	}
	var release struct {
		Tag        string         `json:"tag_name"`
		Name       string         `json:"name"`
		Draft      bool           `json:"draft"`
		Prerelease bool           `json:"prerelease"`
		Assets     []releaseAsset `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release); err != nil {
		return result, fmt.Errorf("無法讀取更新資訊。")
	}
	if release.Draft || release.Prerelease {
		result.Message = "目前沒有可取得的正式發行版本。"
		return result, nil
	}
	result.Version = release.Tag
	if versionKey(result.Version) == "" {
		result.Version = release.Name
	}
	if versionKey(result.Version) == "" || versionKey(currentVersion()) == "" {
		result.Message = "發行版本格式無法比較，請前往下載頁確認。"
		return result, nil
	}
	result.Available = versionKey(result.Version) > versionKey(currentVersion())
	result.Message = "目前已是最新版本。"
	if result.Available {
		result.Message = "發現新版本，可前往下載。"
	}
	system := runtime.GOOS
	extensions := []string{"-setup.exe", ".zip"}
	if system == "darwin" {
		system = "macos"
		extensions = []string{".dmg"}
	}
	// 對外採 x64，保留舊 amd64 套件的相容性；安裝程式仍優先於 ZIP。
	architectures := []string{runtime.GOARCH}
	if runtime.GOARCH == "amd64" {
		architectures = []string{"x64", "amd64"}
	}
	for _, ext := range extensions {
		for _, architecture := range architectures {
			suffix := "-" + system + "-" + architecture + ext
			for _, asset := range release.Assets {
				if strings.HasPrefix(asset.Name, "YourDesk-") && strings.HasSuffix(asset.Name, suffix) && validReleaseAsset(asset) {
					result.Asset = asset
					break
				}
			}
			if result.Asset.URL != "" {
				break
			}
		}
		if result.Asset.URL != "" {
			break
		}
	}
	if result.Available && result.Asset.URL == "" {
		result.Message = "新版尚未提供此系統的安裝包。"
	}
	return result, nil
}
func validReleaseAsset(asset releaseAsset) bool {
	parsed, err := url.Parse(asset.URL)
	return err == nil && parsed.Scheme == "https" && parsed.Host == "github.com" && parsed.User == nil && strings.HasPrefix(parsed.Path, "/VaderChen/YourDesk/releases/download/") && asset.Name != "" && filepath.Base(asset.Name) == asset.Name && !strings.ContainsAny(asset.Name, "/\\:") && asset.Size > 0 && asset.Size <= maxUpdateSize
}

// 下載驗證完成後交給獨立更新程序；準備成功才結束 APP。
func (u *updateManager) startDownload(onOpened func()) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.ctx.Err() != nil {
		return fmt.Errorf("下載失敗，請稍後重試。")
	}
	if u.decision != nil {
		return nil
	}
	if u.state.Downloading || u.state.Opening {
		return nil
	}
	asset := u.state.Asset
	if !u.state.Available || !validReleaseAsset(asset) {
		return fmt.Errorf("新版尚未提供此系統的安裝包。")
	}
	path := u.state.DownloadPath
	u.state.Downloading = path == ""
	if path == "" {
		u.state.DownloadBytes = 0
	} else {
		u.state.DownloadBytes = asset.Size
	}
	u.state.Opening = path != ""
	u.state.DownloadError, u.state.OpenError = "", ""
	u.state.NotifiedVersion = u.state.Version
	u.saveLocked()
	u.downloads.Add(1)
	go func() {
		defer u.downloads.Done()
		var err error
		if path == "" {
			path, err = downloadRelease(u.ctx, asset, func(n int64) {
				u.mu.Lock()
				u.state.DownloadBytes = min(n, asset.Size)
				u.mu.Unlock()
			})
		} else {
			err = verifyDownloadedRelease(path, asset)
			if err != nil {
				path = ""
			}
		}
		u.mu.Lock()
		u.state.Downloading = false
		u.state.DownloadPath = path
		u.state.Opening = err == nil
		if err != nil {
			u.state.DownloadError = err.Error()
		}
		u.saveLocked()
		u.mu.Unlock()
		if err != nil {
			return
		}
		{
			u.mu.Lock()
			u.state.Opening = false
			u.state.InstallAt = time.Now().Add(10 * time.Second).UnixMilli()
			decision := make(chan bool, 1)
			u.decision = decision
			u.mu.Unlock()
			timer := time.NewTimer(10 * time.Second)
			install := true
			select {
			case install = <-decision:
			case <-timer.C:
			case <-u.ctx.Done():
				install = false
			}
			timer.Stop()
			u.mu.Lock()
			// 倒數到期同時收到取消時，優先採用使用者決定。
			select {
			case install = <-decision:
			default:
			}
			u.decision = nil
			u.state.InstallAt = 0
			u.state.Opening = install
			u.saveLocked()
			u.mu.Unlock()
			if !install {
				return
			}
		}
		err = prepareAutomaticUpdate(u.ctx, path)
		u.mu.Lock()
		// 交接成功後維持忙碌，直到 APP 退出，避免顯示可重複啟動的重試按鈕。
		u.state.Opening = err == nil
		if err != nil {
			slog.Warn("自動更新準備失敗", "error", err)
			u.state.OpenError = "無法自動安裝：" + err.Error()
		}
		u.saveLocked()
		u.mu.Unlock()
		if err == nil && onOpened != nil {
			onOpened()
		}
	}()
	return nil
}

// 重新開啟前確認持久化路徑仍指向完整、符合目前 Release 的套件。
func verifyDownloadedRelease(path string, asset releaseAsset) error {
	invalid := fmt.Errorf("下載檔案遺失或驗證失敗，請重新下載。")
	home, err := os.UserHomeDir()
	if err != nil || path != filepath.Join(home, "Downloads", "YourDesk", asset.Name) {
		return invalid
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.Size {
		return invalid
	}
	file, err := os.Open(path)
	if err != nil {
		return invalid
	}
	defer file.Close()
	if asset.Digest != "" {
		hash := sha256.New()
		n, err := io.Copy(hash, io.LimitReader(file, asset.Size+1))
		if err != nil || n != asset.Size || !strings.EqualFold(asset.Digest, "sha256:"+hex.EncodeToString(hash.Sum(nil))) {
			return invalid
		}
	}
	return nil
}

// 僅更新記憶體中的實際下載量，避免每一片資料都寫入設定檔。
type downloadProgress struct {
	bytes  int64
	last   time.Time
	report func(int64)
}

func (p *downloadProgress) Write(data []byte) (int, error) {
	p.bytes += int64(len(data))
	if p.report != nil && time.Since(p.last) >= 100*time.Millisecond {
		p.report(p.bytes)
		p.last = time.Now()
	}
	return len(data), nil
}

func downloadRelease(ctx context.Context, asset releaseAsset, progress func(int64)) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("無法建立下載檔案。")
	}
	dir := filepath.Join(home, "Downloads", "YourDesk")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("無法建立下載檔案。")
	}
	file, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return "", fmt.Errorf("無法建立下載檔案。")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", asset.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "YourDesk")
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		host := req.URL.Hostname()
		trusted := host == "github.com" || strings.HasSuffix(host, ".githubusercontent.com")
		if len(via) >= 10 || req.URL.Scheme != "https" || !trusted {
			return fmt.Errorf("無效的下載轉址。")
		}
		return nil
	}}
	response, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("下載失敗，請稍後重試。")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("下載失敗，請稍後重試。")
	}
	hash := sha256.New()
	counter := &downloadProgress{report: progress}
	size, err := io.Copy(io.MultiWriter(file, hash, counter), io.LimitReader(response.Body, asset.Size+1))
	if progress != nil {
		progress(counter.bytes)
	}
	if err != nil || size != asset.Size {
		return "", fmt.Errorf("下載檔案不完整，請重試。")
	}
	if asset.Digest != "" && (!strings.HasPrefix(asset.Digest, "sha256:") || !strings.EqualFold(strings.TrimPrefix(asset.Digest, "sha256:"), hex.EncodeToString(hash.Sum(nil)))) {
		return "", fmt.Errorf("下載檔案驗證失敗，請重試。")
	}
	if err = file.Sync(); err != nil {
		return "", fmt.Errorf("無法儲存下載檔案。")
	}
	if err = file.Close(); err != nil {
		return "", fmt.Errorf("無法儲存下載檔案。")
	}
	path := filepath.Join(dir, asset.Name)
	if err = os.Rename(file.Name(), path); err != nil {
		return "", fmt.Errorf("無法儲存下載檔案。")
	}
	return path, nil
}
func (s *server) handleUpdates(w http.ResponseWriter, r *http.Request) bool {
	if s.updater == nil {
		return false
	}
	switch {
	case r.URL.Path == "/api/updates" && r.Method == "GET":
		respond(w, 200, s.updater.check(r.Context(), true, r.URL.Query().Get("force") == "true"))
	case r.URL.Path == "/api/updates/state" && r.Method == "GET":
		respond(w, 200, s.updater.snapshot())
	case r.URL.Path == "/api/updates/ack" && r.Method == "POST":
		var request struct {
			Version string `json:"version"`
		}
		if err := decode(w, r, &request); err != nil {
			fail(w, err)
			return true
		}
		s.updater.mu.Lock()
		if request.Version == s.updater.state.Version {
			s.updater.state.NotifiedVersion = request.Version
		}
		s.updater.saveLocked()
		s.updater.mu.Unlock()
		respond(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/updates/install-now" && r.Method == "POST":
		s.updater.mu.Lock()
		if s.updater.decision != nil {
			select {
			case s.updater.decision <- true:
			default:
			}
		}
		s.updater.mu.Unlock()
		respond(w, 200, s.updater.snapshot())
	case r.URL.Path == "/api/updates/cancel" && r.Method == "POST":
		s.updater.mu.Lock()
		if s.updater.decision != nil {
			// 取消取代尚未處理的立即更新要求。
			select {
			case <-s.updater.decision:
			default:
			}
			s.updater.decision <- false
		}
		s.updater.mu.Unlock()
		respond(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/updates/quit" && r.Method == "POST":
		if s.closeApplication == nil {
			fail(w, fmt.Errorf("無法結束本程式"))
			return true
		}
		respond(w, 200, map[string]bool{"ok": true})
		s.closeApplication()
	case r.URL.Path == "/api/updates/download" && r.Method == "POST":
		if err := s.updater.startDownload(func() {
			if s.closeApplication != nil {
				s.closeApplication()
			}
		}); err != nil {
			fail(w, err)
		} else {
			respond(w, 200, s.updater.snapshot())
		}
	default:
		return false
	}
	return true
}
