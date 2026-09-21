//go:build cgo && (darwin || windows)

package clientui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in native smoke test. Both peers are isolated loopback fixtures: metadata
// always fails, so this test never creates any files in the user's Downloads.
func TestNativeFilesWindows(t *testing.T) {
	binary := os.Getenv("YOURDESK_FILES_WINDOW_BINARY")
	if binary == "" {
		t.Skip("需指定 YOURDESK_FILES_WINDOW_BINARY 原生視窗測試執行檔")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	type notice struct{ id, status string }
	notices := make(chan notice, 16)
	pending := make(chan struct{}, 1)
	var calls atomic.Int32
	const token = "isolated-native-files-test-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, `<!doctype html><title>YourDesk 檔案視窗測試</title><p>隔離檔案視窗測試，不下載真實檔案，會自動關閉。</p><script src="/test.js"></script>`)
		case "/test.js":
			w.Header().Set("Content-Type", "application/javascript")
			fmt.Fprint(w, `const id=new URLSearchParams(location.hash.slice(1)).get('session');
const report=status=>fetch('/notice?id='+encodeURIComponent(id)+'&status='+encodeURIComponent(status));
(async()=>{
 try {
  for(const name of ['yourdeskStartFile','yourdeskPrepareFile','yourdeskPauseFile','yourdeskResumeFile','yourdeskCancelFile','yourdeskCloseFiles','yourdeskFilesSession','yourdeskFilesReady'])
   if(typeof window[name]!=='function')throw new Error('missing '+name);
  window.addEventListener('yourdesk-request-close',()=>void window.yourdeskCloseFiles());
  await window.yourdeskFilesReady();
  if((await window.yourdeskFilesSession()).instance!=='fixture')throw new Error('initial session snapshot is invalid');
  await report('ready');
  if(id==='no-prepare') {
   await report('closing');
   await window.yourdeskCloseFiles();
   return;
  }
  let rejected=false;
  try { await window.yourdeskPrepareFile('missing-test-file'); }
  catch(error) { rejected=String(error).includes('isolated fixture: missing file'); }
  if(!rejected)throw new Error('download did not reject the remote error');
  await window.yourdeskCancelFile();
  await report('rejected');
  let settled=false;
  const pending=window.yourdeskPrepareFile('cancel-test-file').then(()=>{settled=true;return false},()=>{settled=true;return true});
  const waiting=await fetch('/wait-pending');
  if(!waiting.ok)throw new Error('pending request did not start');
  const paused=await window.yourdeskPauseFile();
  if(!paused.paused||paused.received!==0)throw new Error('native pause reply is invalid');
  const rebound=new Promise((resolve,reject)=>window.addEventListener('yourdesk-files-session',event=>event.detail.instance==='fixture-two'?resolve():reject(new Error('invalid rebind')),{once:true}));
  await report('paused');
  await rebound;
  if((await window.yourdeskFilesSession()).instance!=='fixture-two')throw new Error('rebound session snapshot is invalid');
  if(settled)throw new Error('pause/rebind rejected preparation');
  await report('rebound');
  const resumed=await window.yourdeskResumeFile();
  if(!resumed.resumed)throw new Error('native resume reply is invalid');
  if(!(await fetch('/wait-pending')).ok)throw new Error('resumed request did not start');
  await window.yourdeskCancelFile();
  if(!await pending)throw new Error('cancelled preparation did not reject');
  await report('cancelled');
 } catch(error) { await report('error: '+String(error)); await window.yourdeskCloseFiles(); }
})();`)
		case "/api/files":
			calls.Add(1)
			var request struct {
				Session, Instance, Action string
				Params                    struct{ Path string }
			}
			if r.Method != http.MethodPost || r.Header.Get("X-YourDesk-Token") != token || json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&request) != nil || request.Session != "remote-error" || (request.Instance != "fixture" && request.Instance != "fixture-two") || request.Action != "stat" {
				select {
				case notices <- notice{"remote-error", "error: unexpected native request"}:
				default:
				}
			}
			if request.Params.Path == "cancel-test-file" {
				select {
				case pending <- struct{}{}:
				default:
				}
				select {
				case <-r.Context().Done():
				case <-ctx.Done():
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"isolated fixture: missing file"}`)
		case "/wait-pending":
			select {
			case <-pending:
				w.WriteHeader(http.StatusNoContent)
			case <-ctx.Done():
				http.Error(w, "pending request not observed", http.StatusRequestTimeout)
			}
		case "/notice":
			select {
			case notices <- notice{r.URL.Query().Get("id"), r.URL.Query().Get("status")}:
			case <-ctx.Done():
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	type exit struct {
		err    error
		output string
	}
	type child struct {
		input io.WriteCloser
		exit  chan exit
	}
	children := make(map[string]child)
	for _, id := range []string{"no-prepare", "remote-error"} {
		cmd := exec.CommandContext(ctx, binary, "--files-window")
		input, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &output
		if err = cmd.Start(); err != nil {
			input.Close()
			t.Fatal(err)
		}
		exited := make(chan exit, 1)
		go func() {
			err := cmd.Wait()
			exited <- exit{err, output.String()}
			close(exited)
		}()
		t.Cleanup(func() {
			input.Close()
			_ = cmd.Process.Kill()
			<-exited
		})
		fragment := url.Values{"token": {token}, "session": {id}, "instance": {"fixture"}, "name": {"Demo"}, "language": {"en"}}
		if err = json.NewEncoder(input).Encode(map[string]string{"url": srv.URL + "/files.html#" + fragment.Encode(), "title": "YourDesk · 檔案視窗測試 " + id}); err != nil {
			t.Fatal(err)
		}
		children[id] = child{input, exited}
	}
	ready := make(map[string]bool)
	rejected := false
	cancelled := false
	paused, rebound := false, false
	for len(ready) < 2 || !rejected || !cancelled || !paused || !rebound {
		select {
		case value := <-notices:
			switch value.status {
			case "ready":
				ready[value.id] = true
			case "rejected":
				rejected = true
			case "cancelled":
				cancelled = true
			case "paused":
				paused = true
				fragment := url.Values{"token": {token}, "session": {"remote-error"}, "instance": {"fixture-two"}, "name": {"Demo"}, "language": {"en"}}
				message := filesWindowMessage{Event: "files-session", URL: srv.URL + "/files.html#" + fragment.Encode(), Title: "YourDesk · 隔離重連測試"}
				if err := json.NewEncoder(children["remote-error"].input).Encode(message); err != nil {
					t.Fatal(err)
				}
			case "rebound":
				rebound = true
			case "closing":
			default:
				t.Fatalf("native %s: %s", value.id, value.status)
			}
		case <-ctx.Done():
			t.Fatal("原生檔案視窗未載入或遠端錯誤未返回")
		}
	}
	waitExit := func(id string) {
		t.Helper()
		select {
		case result := <-children[id].exit:
			if result.err != nil {
				t.Fatalf("%s 視窗失敗: %v\n%s", id, result.err, result.output)
			}
		case <-ctx.Done():
			t.Fatalf("%s 視窗未關閉", id)
		}
	}
	waitExit("no-prepare")
	select {
	case result := <-children["remote-error"].exit:
		t.Fatalf("第二個視窗受到影響: %v\n%s", result.err, result.output)
	default:
	}
	children["remote-error"].input.Close()
	waitExit("remote-error")
	if calls.Load() != 3 {
		t.Fatalf("expected initial rejected, paused and resumed stat requests, got %d", calls.Load())
	}
	t.Log("PASS: 原生視窗獨立載入與關閉；等待網路時可暫停、同窗重綁、明確續傳與取消；父管線 EOF 可回收；未建立下載檔案")
}
