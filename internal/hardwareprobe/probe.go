// Package hardwareprobe 在背景子程序中偵測能力，不參與連線後端切換。
package hardwareprobe

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"
	"yourdesk/internal/childprocess"
)

const helperFlag = "--hardware-probe"
const Workers = 4

type job struct {
	Phase  string `json:"phase,omitempty"`
	Codec  string `json:"codec"`
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type Result struct {
	Key        string          `json:"key"`
	State      string          `json:"state"`
	DurationMS float64         `json:"durationMS"`
	Data       json.RawMessage `json:"data,omitempty"`
}
type State struct {
	Status     string   `json:"status"`
	Workers    int      `json:"workers"`
	Completed  int      `json:"completed"`
	Total      int      `json:"total"`
	DurationMS float64  `json:"durationMS"`
	Results    []Result `json:"results"`
}

var once sync.Once
var done = make(chan struct{})
var mu sync.RWMutex
var state = State{Status: "not-started", Workers: Workers}
var deepState = State{Status: "not-started", Workers: Workers}
var lifetime context.Context
var deepDone chan struct{}

// Start 只啟動協調 goroutine；不等待程序建立、硬體查詢或完成。
func Start(ctx context.Context) {
	once.Do(func() {
		mu.Lock()
		lifetime = ctx
		mu.Unlock()
		go func() { defer close(done); detect(ctx) }()
	})
}

// StartDeep 重新執行完整平台清單，不覆寫啟動策略，也不允許重複執行。
func StartDeep() bool {
	mu.Lock()
	defer mu.Unlock()
	if lifetime == nil || lifetime.Err() != nil || state.Status == "running" || state.Status == "not-started" || deepState.Status == "running" {
		return false
	}
	deepState = State{Status: "running", Workers: Workers, Total: len(platformJobs())}
	finished := make(chan struct{})
	deepDone = finished
	ctx := lifetime
	go func() { defer close(finished); detectInto(ctx, &deepState) }()
	return true
}

func DeepSnapshot() State {
	mu.RLock()
	defer mu.RUnlock()
	s := deepState
	s.Results = append([]Result(nil), deepState.Results...)
	for i := range s.Results {
		s.Results[i].Data = append(json.RawMessage(nil), s.Results[i].Data...)
	}
	return s
}

// WaitForShutdown 僅在取消後的退出路徑呼叫，給程序監督者時間終止 helper。
// 不在 APP 開啟或狀態讀取時等待。
func WaitForShutdown() {
	defer func() {
		mu.RLock()
		finished := deepDone
		mu.RUnlock()
		if finished != nil {
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			select {
			case <-finished:
			case <-timer.C:
			}
		}
	}()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}

// Snapshot 僅複製記憶體結果，不呼叫硬體 API 或等待探測。
func Snapshot() State {
	mu.RLock()
	defer mu.RUnlock()
	s := state
	s.Results = append([]Result(nil), state.Results...)
	for i := range s.Results {
		s.Results[i].Data = append(json.RawMessage(nil), s.Results[i].Data...)
	}
	return s
}

func detect(ctx context.Context) {
	detectInto(ctx, &state)
}

func detectInto(ctx context.Context, state *State) {
	start := time.Now()
	jobs := platformJobs()
	mu.Lock()
	state.Status = "running"
	state.Total = len(jobs)
	state.Results = make([]Result, len(jobs))
	for i, j := range jobs {
		state.Results[i] = Result{Key: key(j), State: "pending"}
	}
	mu.Unlock()
	executable, err := os.Executable()
	if err != nil {
		mu.Lock()
		state.Status = "failed"
		state.DurationMS = float64(time.Since(start).Microseconds()) / 1000
		mu.Unlock()
		slog.Warn("背景硬體偵測無法取得執行檔", "error", err)
		return
	}
	queue := make(chan int, len(jobs))
	for i := range jobs {
		queue <- i
	}
	close(queue)
	var wg sync.WaitGroup
	for worker := 0; worker < Workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range queue {
				result := execute(ctx, executable, jobs[i])
				mu.Lock()
				state.Results[i] = result
				state.Completed++
				state.DurationMS = float64(time.Since(start).Microseconds()) / 1000
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	mu.Lock()
	state.Status = "complete"
	if ctx.Err() != nil {
		state.Status = "cancelled"
	} else {
		for _, r := range state.Results {
			if r.State != "complete" {
				state.Status = "partial"
				break
			}
		}
	}
	state.DurationMS = float64(time.Since(start).Microseconds()) / 1000
	status, elapsed := state.Status, state.DurationMS
	mu.Unlock()
	slog.Info("背景硬體偵測結束", "status", status, "durationMS", elapsed, "workers", Workers, "jobs", len(jobs))
}
func key(j job) string {
	if j.Codec == "" {
		return "inventory"
	}
	return fmt.Sprintf("%s/%s/%dx%d", j.Codec, j.Format, j.Width, j.Height) + phaseSuffix(j.Phase)
}

func phaseSuffix(phase string) string {
	if phase != "" {
		return "/" + phase
	}
	return ""
}

// 限制 helper 輸出大小，避免異常驅動訊息無限制配置記憶體。
type limitedOutput struct{ data []byte }

func (b *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (1 << 20) - len(b.data)
	if remaining > 0 {
		b.data = append(b.data, p[:min(remaining, n)]...)
	}
	return n, nil
}
func execute(parent context.Context, executable string, j job) Result {
	r := Result{Key: key(j), State: "cancelled"}
	if parent.Err() != nil {
		return r
	}
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	args := []string{helperFlag, "inventory"}
	if j.Codec != "" {
		args = []string{helperFlag, j.Codec, j.Format, strconv.Itoa(j.Width), strconv.Itoa(j.Height)}
		if j.Phase != "" {
			args = append(args, j.Phase)
		}
	}
	cmd := childprocess.CommandContext(ctx, executable, args...)
	cmd.WaitDelay = time.Second
	var out limitedOutput
	cmd.Stdout = &out
	// 不繼承 stdin、連線密碼或 UI 控制管線；stderr 不回送使用者介面。
	err := cmd.Run()
	r.DurationMS = float64(time.Since(start).Microseconds()) / 1000
	var envelope map[string]json.RawMessage
	parseErr := json.Unmarshal(out.data, &envelope)
	var helperStatus string
	_ = json.Unmarshal(envelope["status"], &helperStatus)
	switch {
	case parent.Err() != nil:
		r.State = "cancelled"
	case ctx.Err() != nil:
		r.State = "timeout"
	case err != nil:
		r.State = "failed"
	case parseErr != nil || envelope == nil:
		r.State = "invalid-result"
	case helperStatus == "probe-failed" || helperStatus == "invalid-request":
		r.State = "failed"
	default:
		r.State = "complete"
		r.Data = out.data
	}
	return r
}

// HandleHelper 必須在旗標解析、UI、服務與單一執行個體鎖之前呼叫。
// 只接受編譯時定義的探測組合，無檔案路徑／shell／外部目標輸入。
func HandleHelper(args []string) bool {
	if len(args) == 0 || args[0] != helperFlag {
		return false
	}
	var chosen *job
	for _, j := range platformJobs() {
		valid := len(args) == 2 && args[1] == "inventory" && j.Codec == ""
		if (len(args) == 5 && j.Phase == "" || len(args) == 6 && args[5] == j.Phase) && j.Codec != "" {
			valid = args[1] == j.Codec && args[2] == j.Format && args[3] == strconv.Itoa(j.Width) && args[4] == strconv.Itoa(j.Height)
		}
		if valid {
			v := j
			chosen = &v
			break
		}
	}
	if chosen == nil {
		fmt.Fprintln(os.Stdout, `{"status":"invalid-request"}`)
		return true
	}
	data, err := platformProbe(*chosen)
	if err != nil {
		fmt.Fprintln(os.Stdout, `{"status":"probe-failed"}`)
		return true
	}
	_, _ = os.Stdout.Write(data)
	return true
}

// 每個方向都是獨立 helper，逾時與失敗不會影響另一個方向。
func splitJobs(cases []job) []job {
	var jobs []job
	for _, j := range cases {
		if j.Codec == "" {
			jobs = append(jobs, j)
			continue
		}
		for _, phase := range []string{"encode", "decode"} {
			v := j
			v.Phase = phase
			jobs = append(jobs, v)
		}
	}
	return jobs
}
