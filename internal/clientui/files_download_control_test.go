package clientui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type controlledDownloadRequest struct {
	Session, Instance, Action string
	Params                    struct {
		Path   string
		Offset int64
		Length int
	}
}

func controlledDownloadFixture(t *testing.T, data []byte, intercept func(http.ResponseWriter, *http.Request, controlledDownloadRequest) bool) *fileDownloadClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request controlledDownloadRequest
		if r.Header.Get("X-YourDesk-Token") != "fixture-token" || json.NewDecoder(r.Body).Decode(&request) != nil || request.Session != "viewer:fixture" {
			http.Error(w, "bad request", 400)
			return
		}
		if intercept != nil && intercept(w, r, request) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Action {
		case "stat":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "fixture.txt", "size": len(data), "modified": "v1", "directory": false})
		case "read":
			start := int(request.Params.Offset)
			end := min(len(data), start+request.Params.Length)
			if start < 0 || start > end || request.Params.Length > 4096 {
				http.Error(w, "bounds", 400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": base64.StdEncoding.EncodeToString(data[start:end]), "bytes": end - start, "nextOffset": end, "eof": end == len(data)})
		default:
			http.Error(w, "bad action", 400)
		}
	}))
	t.Cleanup(server.Close)
	return &fileDownloadClient{endpoint: server.URL + "/api/files", token: "fixture-token", session: "viewer:fixture", instance: "instance-one", downloads: t.TempDir(), client: server.Client()}
}

type controlledDownloadResult struct {
	out preparedFile
	err error
}

func startControlledDownload(t *testing.T, client *fileDownloadClient) (*fileDownloadControl, <-chan controlledDownloadResult) {
	t.Helper()
	control := newFileDownloadControl(context.Background(), nil)
	done := make(chan controlledDownloadResult, 1)
	go func() {
		out, err := client.prepareControlled(control, "fixture.txt")
		done <- controlledDownloadResult{out, err}
		close(done)
	}()
	t.Cleanup(func() {
		control.stop()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("download worker did not stop")
		}
	})
	return control, done
}

func waitControlledState(t *testing.T, control *fileDownloadControl, state string) fileProgress {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		value := control.snapshot()
		if value.State == state {
			return value
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatalf("expected state %s, got %+v", state, value)
		}
	}
}

func awaitControlledResult(t *testing.T, done <-chan controlledDownloadResult) controlledDownloadResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("download did not finish")
		return controlledDownloadResult{}
	}
}

func checkControlledBytes(t *testing.T, result controlledDownloadResult, expected []byte) {
	t.Helper()
	if result.err != nil {
		t.Fatal(result.err)
	}
	actual, err := os.ReadFile(result.out.Path)
	if err != nil || !bytes.Equal(actual, expected) {
		t.Fatalf("resumed bytes differ: %v (got %d, want %d)", err, len(actual), len(expected))
	}
}

func TestDownloadPauseResumeKeepsConfirmedOffset(t *testing.T) {
	data := bytes.Repeat([]byte("abc123"), 2400)
	reading, cancelled := make(chan struct{}), make(chan struct{})
	var blocked atomic.Bool
	var stats atomic.Int32
	client := controlledDownloadFixture(t, data, func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
		if in.Action == "stat" {
			stats.Add(1)
		}
		if in.Action == "read" && in.Params.Offset == 4096 && blocked.CompareAndSwap(false, true) {
			close(reading)
			<-r.Context().Done()
			close(cancelled)
			return true
		}
		return false
	})
	control, done := startControlledDownload(t, client)
	select {
	case <-reading:
	case <-time.After(3 * time.Second):
		t.Fatal("second block was not requested")
	}
	paused, err := control.pause()
	if err != nil || paused.Received != 4096 || paused.Total != int64(len(data)) {
		t.Fatalf("pause offset: %+v %v", paused, err)
	}
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("pause did not cancel HTTP")
	}
	select {
	case result := <-done:
		t.Fatalf("pause finished the promise: %+v", result)
	default:
	}
	files, err := filepath.Glob(filepath.Join(client.downloads, ".yourdesk-part-*"))
	if err != nil || len(files) != 1 {
		t.Fatalf("partial missing: %v %v", files, err)
	}
	info, err := os.Stat(files[0])
	if err != nil || info.Size() != 4096 {
		t.Fatal("confirmed partial offset lost", err)
	}
	if err = control.resume(); err != nil {
		t.Fatal(err)
	}
	checkControlledBytes(t, awaitControlledResult(t, done), data)
	if stats.Load() < 3 {
		t.Fatal("resume did not revalidate metadata")
	}
	if state := control.snapshot(); state.State != "complete" || state.Received != int64(len(data)) {
		t.Fatalf("completion: %+v", state)
	}
}

func TestDownloadInterruptedRebindRequiresExplicitResume(t *testing.T) {
	data := bytes.Repeat([]byte("resume"), 1900)
	var failed atomic.Bool
	var requests atomic.Int32
	var lock sync.Mutex
	var instances []string
	client := controlledDownloadFixture(t, data, func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
		requests.Add(1)
		lock.Lock()
		instances = append(instances, in.Instance)
		lock.Unlock()
		if in.Action == "read" && in.Params.Offset == 4096 && failed.CompareAndSwap(false, true) {
			http.Error(w, "disconnected", 503)
			return true
		}
		return false
	})
	control, done := startControlledDownload(t, client)
	value := waitControlledState(t, control, "interrupted")
	if value.Received != 4096 || value.Speed != 0 {
		t.Fatalf("interrupted progress: %+v", value)
	}
	before := requests.Load()
	control.interrupt(nil)
	address := strings.TrimSuffix(client.endpoint, "/api/files") + "/files.html#token=fixture-token&session=viewer%3Afixture&instance=instance-two&name=Demo&language=en"
	if _, err := client.rebind(address); err != nil {
		t.Fatal(err)
	}
	if control.snapshot().State != "interrupted" || requests.Load() != before {
		t.Fatal("rebind resumed implicitly")
	}
	select {
	case result := <-done:
		t.Fatalf("connection loss rejected promise: %+v", result)
	default:
	}
	if err := control.resume(); err != nil {
		t.Fatal(err)
	}
	checkControlledBytes(t, awaitControlledResult(t, done), data)
	lock.Lock()
	defer lock.Unlock()
	for _, instance := range instances[before:] {
		if instance != "instance-two" {
			t.Fatalf("old session reused: %q", instance)
		}
	}
}

func TestDownloadSourceChangedOnResumeCleansPartial(t *testing.T) {
	data := bytes.Repeat([]byte{1}, 12000)
	var lost, changed atomic.Bool
	client := controlledDownloadFixture(t, data, func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
		if in.Action == "read" && in.Params.Offset == 4096 && lost.CompareAndSwap(false, true) {
			http.Error(w, "offline", 503)
			return true
		}
		if in.Action == "stat" && changed.Load() {
			fmt.Fprintf(w, `{"name":"fixture.txt","size":%d,"modified":"v2"}`, len(data))
			return true
		}
		return false
	})
	control, done := startControlledDownload(t, client)
	waitControlledState(t, control, "interrupted")
	changed.Store(true)
	if err := control.resume(); err != nil {
		t.Fatal(err)
	}
	result := awaitControlledResult(t, done)
	if result.err == nil || !strings.Contains(result.err.Error(), "來源檔案已變更") {
		t.Fatalf("changed source accepted: %v", result.err)
	}
	entries, err := downloadPartials(client.downloads)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial retained after fatal source change: %v %v", entries, err)
	}
}

func TestDownloadCancelWhileInterruptedRemovesOnlyPartial(t *testing.T) {
	client := controlledDownloadFixture(t, bytes.Repeat([]byte{2}, 12000), func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
		if in.Action == "read" && in.Params.Offset == 4096 {
			http.Error(w, "offline", 503)
			return true
		}
		return false
	})
	marker := filepath.Join(client.downloads, "existing.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	control, done := startControlledDownload(t, client)
	waitControlledState(t, control, "interrupted")
	control.stop()
	if result := awaitControlledResult(t, done); result.err == nil {
		t.Fatal("cancel succeeded")
	}
	entries, err := downloadPartials(client.downloads)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancelled partial retained: %v %v", entries, err)
	}
	if content, err := os.ReadFile(marker); err != nil || string(content) != "keep" {
		t.Fatal("unrelated file changed")
	}
}

func TestDownloadFinalMetadataInterruptCanResume(t *testing.T) {
	data := bytes.Repeat([]byte("final"), 900)
	var stats atomic.Int32
	client := controlledDownloadFixture(t, data, func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
		if in.Action == "stat" && stats.Add(1) == 2 {
			http.Error(w, "offline", 503)
			return true
		}
		return false
	})
	control, done := startControlledDownload(t, client)
	value := waitControlledState(t, control, "interrupted")
	if value.Received != int64(len(data)) {
		t.Fatalf("final offset lost: %+v", value)
	}
	if err := control.resume(); err != nil {
		t.Fatal(err)
	}
	checkControlledBytes(t, awaitControlledResult(t, done), data)
}

func TestFilesWindowProductionURLAndRebindIsolation(t *testing.T) {
	base := "http://127.0.0.1:12345/files.html"
	params := url.Values{"token": {"secret"}, "session": {"viewer:fixture"}, "instance": {"one"}, "name": {"測試電腦"}, "language": {"zh-TW"}}
	address := base + "#" + params.Encode()
	client := &fileDownloadClient{endpoint: "http://127.0.0.1:12345/api/files", token: "secret", session: "viewer:fixture", instance: "one"}
	if _, _, err := parseFilesAddress(address); err != nil {
		t.Fatal(err)
	}
	params.Set("instance", "two")
	if instance, err := client.rebind(base + "#" + params.Encode()); err != nil || instance != "two" {
		t.Fatal(instance, err)
	}
	for _, bad := range []string{strings.Replace(address, ":12345/", ":54321/", 1), strings.Replace(address, "token=secret", "token=other", 1), strings.Replace(address, "viewer%3Afixture", "viewer%3Aother", 1), address + "&token=again", address + "&extra=value"} {
		if _, err := client.rebind(bad); err == nil {
			t.Fatalf("rebound unrelated session: %q", bad)
		}
	}
	if client.instance != "two" {
		t.Fatal("invalid message changed instance")
	}
	initial, _ := json.Marshal(filesWindowMessage{URL: address, Title: "Demo"})
	update, _ := json.Marshal(filesWindowMessage{Event: "files-session", URL: address, Title: "Demo"})
	if _, err := decodeFilesWindowMessage(initial, true); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeFilesWindowMessage(update, false); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{initial, append(update, []byte(` {}`)...), []byte(strings.Repeat("x", 16385))} {
		if _, err := decodeFilesWindowMessage(bad, false); err == nil {
			t.Fatal("invalid update accepted")
		}
	}
}

func TestDownloadReplacementDuringPauseIsPreserved(t *testing.T) {
	for _, action := range []string{"resume", "cancel", "destination"} {
		t.Run(action, func(t *testing.T) {
			var failed atomic.Bool
			client := controlledDownloadFixture(t, bytes.Repeat([]byte{3}, 9000), func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
				if in.Action == "read" && in.Params.Offset == 4096 && failed.CompareAndSwap(false, true) {
					http.Error(w, "offline", 503)
					return true
				}
				return false
			})
			control, done := startControlledDownload(t, client)
			waitControlledState(t, control, "interrupted")
			partials, err := filepath.Glob(filepath.Join(client.downloads, ".yourdesk-part-*"))
			if err != nil || len(partials) != 1 {
				t.Fatal("partial missing", err)
			}
			foreign := partials[0]
			if action == "destination" {
				foreign = filepath.Join(filepath.Dir(foreign), "fixture.txt")
			} else if err := os.Rename(foreign, foreign+".kept"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(foreign, []byte("external replacement"), 0600); err != nil {
				t.Fatal(err)
			}
			if action == "cancel" {
				control.stop()
			} else if err := control.resume(); err != nil {
				t.Fatal(err)
			}
			if result := awaitControlledResult(t, done); result.err == nil {
				t.Fatal("external replacement accepted")
			}
			if content, err := os.ReadFile(foreign); err != nil || string(content) != "external replacement" {
				t.Fatal("external replacement overwritten or deleted", err)
			}
		})
	}
}

func TestDownloadInitialSessionLossPausesBeforeCreatingFiles(t *testing.T) {
	var unavailable atomic.Bool
	unavailable.Store(true)
	client := controlledDownloadFixture(t, []byte("reconnected"), func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
		if unavailable.Load() {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"code":"files_session_unavailable","error":"unavailable"}`)
			return true
		}
		return false
	})
	control, done := startControlledDownload(t, client)
	waitControlledState(t, control, "interrupted")
	entries, err := os.ReadDir(client.downloads)
	if err != nil || len(entries) != 0 {
		t.Fatal("created download before metadata", err)
	}
	select {
	case result := <-done:
		t.Fatalf("initial disconnect rejected: %+v", result)
	default:
	}
	unavailable.Store(false)
	if err := control.resume(); err != nil {
		t.Fatal(err)
	}
	checkControlledBytes(t, awaitControlledResult(t, done), []byte("reconnected"))
}

func assertFailedDownloadCleaned(t *testing.T, client *fileDownloadClient, control *fileDownloadControl, done <-chan controlledDownloadResult, message string) {
	t.Helper()
	result := awaitControlledResult(t, done)
	if result.err == nil || !strings.Contains(result.err.Error(), message) {
		t.Fatalf("permanent failure was not returned: %+v", result)
	}
	if state := control.snapshot().State; state != "failed" {
		t.Fatalf("permanent failure left state %q", state)
	}
	entries, err := downloadPartials(client.downloads)
	if !errors.Is(err, os.ErrNotExist) && (err != nil || len(entries) != 0) {
		t.Fatalf("permanent failure retained partial: %v %v", entries, err)
	}
}

func TestDownloadPermanentReadFailureReturnsErrorAndCleansPartial(t *testing.T) {
	for _, status := range []int{400, 403, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var failures atomic.Int32
			client := controlledDownloadFixture(t, bytes.Repeat([]byte{1}, 9000), func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
				if in.Action == "read" && in.Params.Offset == 4096 {
					failures.Add(1)
					w.WriteHeader(status)
					fmt.Fprint(w, `{"error":"source permission denied","code":"files_operation_failed"}`)
					return true
				}
				return false
			})
			marker := filepath.Join(client.downloads, "existing.txt")
			if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			control, done := startControlledDownload(t, client)
			assertFailedDownloadCleaned(t, client, control, done, "source permission denied")
			if failures.Load() != 1 || control.snapshot().Received != 4096 {
				t.Fatal("permanent failure was retried or lost its confirmed progress")
			}
			if err := control.resume(); err == nil {
				t.Fatal("completed failure still resumable")
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
				t.Fatal("unrelated file changed", err)
			}
		})
	}
}

func TestDownloadPermanentStatFailureReturnsAtEveryCheckpoint(t *testing.T) {
	for _, phase := range []string{"initial", "resume", "final"} {
		t.Run(phase, func(t *testing.T) {
			var stats atomic.Int32
			var lost atomic.Bool
			client := controlledDownloadFixture(t, bytes.Repeat([]byte{2}, 9000), func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
				if phase == "resume" && in.Action == "read" && in.Params.Offset == 4096 && lost.CompareAndSwap(false, true) {
					w.WriteHeader(503)
					return true
				}
				if in.Action == "stat" {
					count := stats.Add(1)
					if (phase == "initial" && count == 1) || (phase != "initial" && count == 2) {
						w.WriteHeader(400)
						fmt.Fprint(w, `{"error":"file no longer exists"}`)
						return true
					}
				}
				return false
			})
			control, done := startControlledDownload(t, client)
			if phase == "resume" {
				waitControlledState(t, control, "interrupted")
				address := strings.TrimSuffix(client.endpoint, "/api/files") + "/files.html#token=fixture-token&session=viewer%3Afixture&instance=instance-two"
				if _, err := client.rebind(address); err != nil {
					t.Fatal(err)
				}
				if err := control.resume(); err != nil {
					t.Fatal(err)
				}
			}
			assertFailedDownloadCleaned(t, client, control, done, "file no longer exists")
		})
	}
}

func TestDownloadTransientErrorClassificationPreservesResume(t *testing.T) {
	for _, failure := range []struct {
		name, code string
		status     int
	}{
		{"session", "files_session_unavailable", 400},
		{"request", "files_request_interrupted", 400},
		{"timeout", "", 408},
		{"busy", "", 429},
		{"service", "", 503},
	} {
		for _, phase := range []string{"initial", "read", "final"} {
			t.Run(failure.name+"/"+phase, func(t *testing.T) {
				data := bytes.Repeat([]byte{3}, 9000)
				var stats atomic.Int32
				var failed atomic.Bool
				client := controlledDownloadFixture(t, data, func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
					count := int32(0)
					if in.Action == "stat" {
						count = stats.Add(1)
					}
					shouldFail := (phase == "initial" && in.Action == "stat" && count == 1) ||
						(phase == "read" && in.Action == "read" && in.Params.Offset == 4096) ||
						(phase == "final" && in.Action == "stat" && count == 2)
					if shouldFail && failed.CompareAndSwap(false, true) {
						w.WriteHeader(failure.status)
						_ = json.NewEncoder(w).Encode(map[string]string{"code": failure.code, "error": "retry after reconnect"})
						return true
					}
					return false
				})
				control, done := startControlledDownload(t, client)
				progress := waitControlledState(t, control, "interrupted")
				want := map[string]int64{"initial": 0, "read": 4096, "final": int64(len(data))}[phase]
				if progress.Received != want || progress.Speed != 0 {
					t.Fatalf("transient progress lost: %+v", progress)
				}
				select {
				case result := <-done:
					t.Fatalf("transient failure settled preparation: %+v", result)
				default:
				}
				if err := control.resume(); err != nil {
					t.Fatal(err)
				}
				checkControlledBytes(t, awaitControlledResult(t, done), data)
			})
		}
	}
}

func TestDownloadPauseRebindDiscardsOldPermanentFailure(t *testing.T) {
	data := bytes.Repeat([]byte{4}, 9000)
	reading, release, replied := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	client := controlledDownloadFixture(t, data, func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
		if in.Action == "read" && in.Params.Offset == 4096 && once.CompareAndSwap(false, true) {
			close(reading)
			<-release
			w.WriteHeader(403)
			fmt.Fprint(w, `{"error":"obsolete session denied"}`)
			close(replied)
			return true
		}
		return false
	})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	control, done := startControlledDownload(t, client)
	select {
	case <-reading:
	case <-time.After(3 * time.Second):
		t.Fatal("read did not start")
	}
	if _, err := control.pause(); err != nil {
		t.Fatal(err)
	}
	address := strings.TrimSuffix(client.endpoint, "/api/files") + "/files.html#token=fixture-token&session=viewer%3Afixture&instance=instance-two"
	if _, err := client.rebind(address); err != nil {
		t.Fatal(err)
	}
	unblock()
	select {
	case <-replied:
	case <-time.After(3 * time.Second):
		t.Fatal("obsolete request did not complete")
	}
	if err := control.resume(); err != nil {
		t.Fatal(err)
	}
	checkControlledBytes(t, awaitControlledResult(t, done), data)
}

func TestDownloadCancelAfterBridgeInterruptionCleansPartial(t *testing.T) {
	client := controlledDownloadFixture(t, bytes.Repeat([]byte{5}, 9000), func(w http.ResponseWriter, r *http.Request, in controlledDownloadRequest) bool {
		if in.Action == "read" && in.Params.Offset == 4096 {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"code":"files_request_interrupted","error":"request interrupted"}`)
			return true
		}
		return false
	})
	control, done := startControlledDownload(t, client)
	waitControlledState(t, control, "interrupted")
	control.stop()
	if result := awaitControlledResult(t, done); !errors.Is(result.err, context.Canceled) {
		t.Fatalf("cancel result: %+v", result)
	}
	entries, err := downloadPartials(client.downloads)
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancel retained partial: %v %v", entries, err)
	}
}

// 只檢查本次下載暫存；目的資料夾可能已含使用者自己的檔案。
func downloadPartials(directory string) ([]string, error) {
	return filepath.Glob(filepath.Join(directory, ".yourdesk-part-*"))
}
