package clientui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesRejectsStaleWindow(t *testing.T) {
	s := &server{children: map[string]*process{"viewer:one": {fileConnection: true, terminalInstance: "new"}}}
	for _, action := range []string{"list", "stat", "read", "begin", "resume", "write", "commit", "cancel", "mkdir", "remove"} {
		r := httptest.NewRequest("POST", "/api/files", strings.NewReader(`{"session":"viewer:one","instance":"old","action":"`+action+`","params":{}}`))
		w := httptest.NewRecorder()
		s.filesAction(w, r)
		if w.Code != 400 {
			t.Fatalf("stale %s accepted: %d", action, w.Code)
		}
	}
}

func TestFilesHTTPAuthAndOrigin(t *testing.T) {
	s := &server{token: "expected", origin: "http://127.0.0.1:1234"}
	for _, path := range []string{"/api/files", "/api/files/window"} {
		for _, tc := range []struct{ token, origin string }{{"", ""}, {"wrong", s.origin}, {"expected", "http://untrusted.test"}} {
			r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
			r.Header.Set("X-YourDesk-Token", tc.token)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			s.api(w, r)
			if w.Code != http.StatusForbidden {
				t.Fatalf("unauthorized request accepted: %s %d", path, w.Code)
			}
		}
	}
}

func TestFilesRejectsForeignModesAndMethods(t *testing.T) {
	for _, p := range []*process{{terminalConnection: true, terminalInstance: "new"}, {fileConnection: true, mcpOwned: true, terminalInstance: "new"}, {terminalInstance: "new"}} {
		s := &server{children: map[string]*process{"viewer:one": p}}
		w := httptest.NewRecorder()
		s.filesAction(w, httptest.NewRequest("POST", "/api/files", strings.NewReader(`{"session":"viewer:one","instance":"new","action":"list","params":{}}`)))
		if w.Code != 400 {
			t.Fatal("foreign mode accepted")
		}
	}
	s := &server{children: map[string]*process{"viewer:one": {fileConnection: true, terminalInstance: "new"}}}
	for _, action := range []string{"shell.run", "terminal.open", "delete", "disconnect"} {
		w := httptest.NewRecorder()
		s.filesAction(w, httptest.NewRequest("POST", "/api/files", strings.NewReader(`{"session":"viewer:one","instance":"new","action":"`+action+`","params":{}}`)))
		if w.Code != 400 {
			t.Fatal("unallowed method accepted")
		}
	}
}

func TestFilesStatusIsLocalAndInstanceBound(t *testing.T) {
	s := &server{children: map[string]*process{"viewer:one": {fileConnection: true, stage: "connected", terminalInstance: "new"}}}
	for _, tc := range []struct {
		instance  string
		connected bool
	}{{"new", true}, {"old", false}, {"", false}} {
		w := httptest.NewRecorder()
		s.filesAction(w, httptest.NewRequest("POST", "/api/files", strings.NewReader(`{"session":"viewer:one","instance":"`+tc.instance+`","action":"status","params":{}}`)))
		var out struct {
			Connected bool `json:"connected"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Connected != tc.connected {
			t.Fatalf("status: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestFilesWindowAddressAndNames(t *testing.T) {
	valid := "http://127.0.0.1:1234/files.html#token=token&session=viewer%3Atest&instance=id"
	if _, _, err := parseFilesAddress(valid); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{strings.Replace(valid, "127.0.0.1", "example.test", 1), strings.Replace(valid, "http:", "https:", 1), strings.Replace(valid, "files.html", "terminal.html", 1), "http://127.0.0.1:1234/files.html", strings.Replace(valid, "127.0.0.1", "user@127.0.0.1", 1)} {
		if _, _, err := parseFilesAddress(address); err == nil {
			t.Fatalf("bad address accepted: %s", address)
		}
	}
	for _, name := range []string{"../secret", "C:ads", "file:stream", "file\\name", "CON.txt", "LPT1", "NUL", "file.", "file ", ".download.part", "\x00", "a\nb"} {
		if name == ".download.part" {
			continue
		}
		if safeDownloadName(name) {
			t.Fatalf("unsafe name accepted: %q", name)
		}
	}
	if !safeDownloadName("範例 👋.txt") {
		t.Fatal("Unicode rejected")
	}
}

func downloadFixture(t *testing.T, content []byte, mode string) *fileDownloadClient {
	t.Helper()
	name := "範例 👋.txt"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-YourDesk-Token") != "test-token" {
			t.Error("missing auth")
		}
		var in struct {
			Session  string
			Instance string
			Action   string
			Params   struct {
				Offset   int64
				Length   int
				Modified string
				Size     int64
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			return
		}
		if in.Session != "viewer:test" || in.Instance != "instance" {
			t.Error("missing session binding")
		}
		w.Header().Set("Content-Type", "application/json")
		switch in.Action {
		case "stat":
			size := int64(len(content))
			if mode == "oversize" {
				size = 2<<30 + 1
			}
			if mode == "traversal" {
				name = "../escape"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"name": name, "size": size, "modified": "2026-01-01T00:00:00Z", "directory": false})
		case "read":
			if mode == "changed" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"changed"}`))
				return
			}
			if in.Params.Length > 4096 || in.Params.Modified == "" || in.Params.Size != int64(len(content)) {
				t.Error("invalid read bounds")
			}
			start := int(in.Params.Offset)
			end := min(len(content), start+in.Params.Length)
			next := int64(end)
			if mode == "offset" {
				next++
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": base64.StdEncoding.EncodeToString(content[start:end]), "bytes": end - start, "nextOffset": next, "eof": end == len(content)})
		default:
			t.Errorf("unexpected operation: %s", in.Action)
		}
	}))
	t.Cleanup(server.Close)
	return &fileDownloadClient{endpoint: server.URL, token: "test-token", session: "viewer:test", instance: "instance", downloads: t.TempDir(), client: server.Client()}
}

func TestFileDownloadBoundedAndPersistent(t *testing.T) {
	for _, size := range []int{0, 1, 4096, 4097, 65539} {
		t.Run(string(rune('A'+size%26)), func(t *testing.T) {
			content := bytes.Repeat([]byte{0x5a}, size)
			client := downloadFixture(t, content, "")
			var progress []fileProgress
			out, err := client.prepare(context.Background(), "sample", func(p fileProgress) { progress = append(progress, p) })
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(out.Path)
			if err != nil || !bytes.Equal(data, content) {
				t.Fatal("content mismatch", err)
			}
			if out.Size != int64(size) || out.Name != "範例 👋.txt" || len(progress) == 0 {
				t.Fatal("metadata missing")
			}
			second, err := client.prepare(context.Background(), "sample", nil)
			if err != nil {
				t.Fatal(err)
			}
			if second.Path == out.Path {
				t.Fatal("second download overwrites source")
			}
			if _, err = os.Stat(out.Path); err != nil {
				t.Fatal("completed source removed")
			}
		})
	}
}

func TestFileDownloadFailureRemovesOnlyOwnPartial(t *testing.T) {
	for _, mode := range []string{"changed", "offset", "oversize", "traversal", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			client := downloadFixture(t, bytes.Repeat([]byte{1}, 8192), mode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := client.prepare(ctx, "sample", func(p fileProgress) {
				if mode == "cancel" && p.Received > 0 {
					cancel()
				}
			})
			if err == nil {
				t.Fatal("expected failure")
			}
			entries, e := os.ReadDir(filepath.Join(client.downloads, "YourDesk"))
			if e != nil && !os.IsNotExist(e) {
				t.Fatal(e)
			}
			if len(entries) != 0 {
				t.Fatal("partial download left behind")
			}
		})
	}
}
