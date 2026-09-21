package filetransfer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testSession(t *testing.T) (*session, string) {
	t.Helper()
	home := t.TempDir()
	s, err := newSession(home, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	return s, home
}

func call(s *session, method string, in any) (any, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	return s.command(context.Background(), method, raw)
}

func mustCall(t *testing.T, s *session, method string, in any) any {
	t.Helper()
	out, err := call(s, method, in)
	if err != nil {
		t.Fatalf("%s failed: %v", method, err)
	}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > 16384 {
		t.Fatalf("unbounded response: %d bytes, %v", len(encoded), err)
	}
	return out
}

func startUpload(t *testing.T, s *session, name string, size int64) string {
	t.Helper()
	return mustCall(t, s, "begin", map[string]any{"path": name, "size": size}).(map[string]any)["id"].(string)
}

func writeChunk(t *testing.T, s *session, id string, offset int64, data []byte) {
	t.Helper()
	out := mustCall(t, s, "write", map[string]any{"id": id, "offset": offset, "data": base64.StdEncoding.EncodeToString(data)}).(map[string]int64)
	if out["nextOffset"] != offset+int64(len(data)) {
		t.Fatal("wrong write offset")
	}
}

func TestTransferUnicodeAndChunkedRead(t *testing.T) {
	s, home := testSession(t)
	mustCall(t, s, "mkdir", map[string]string{"path": "相簿 😀"})
	name := "相簿 😀/測試 檔案.txt"
	data := bytes.Repeat([]byte{0, 1, 2, 128, 255}, 2300)
	id := startUpload(t, s, name, int64(len(data)))
	for offset := 0; offset < len(data); offset += ChunkSize {
		writeChunk(t, s, id, int64(offset), data[offset:min(offset+ChunkSize, len(data))])
	}
	if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(name))); !os.IsNotExist(err) {
		t.Fatal("uncommitted target became visible")
	}
	mustCall(t, s, "commit", map[string]string{"id": id})
	got, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(name)))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("bad committed content: %v", err)
	}
	info := mustCall(t, s, "stat", map[string]string{"path": name}).(Entry)
	if info.Name != "測試 檔案.txt" || info.Path != name || info.Size != int64(len(data)) || info.Directory {
		t.Fatalf("bad stat: %+v", info)
	}
	var downloaded []byte
	for offset := int64(0); offset < info.Size; {
		out := mustCall(t, s, "read", map[string]any{"path": name, "offset": offset, "length": ChunkSize, "modified": info.Modified, "size": info.Size}).(map[string]any)
		chunk, err := base64.StdEncoding.DecodeString(out["data"].(string))
		if err != nil {
			t.Fatal(err)
		}
		downloaded = append(downloaded, chunk...)
		offset = out["nextOffset"].(int64)
		if out["bytes"].(int) != len(chunk) || out["eof"].(bool) != (offset == info.Size) {
			t.Fatal("bad read metadata")
		}
	}
	if !bytes.Equal(downloaded, data) || len(s.uploads) != 0 || s.reserved != 0 {
		t.Fatal("content or upload accounting differs")
	}
}

func TestEmptyAndCancel(t *testing.T) {
	s, home := testSession(t)
	id := startUpload(t, s, "empty", 0)
	mustCall(t, s, "commit", map[string]string{"id": id})
	info := mustCall(t, s, "stat", map[string]string{"path": "empty"}).(Entry)
	out := mustCall(t, s, "read", map[string]any{"path": "empty", "size": 0, "modified": info.Modified}).(map[string]any)
	if out["data"] != "" || out["eof"] != true {
		t.Fatal("empty read failed")
	}
	id = startUpload(t, s, "cancelled", 123)
	writeChunk(t, s, id, 0, []byte("123"))
	mustCall(t, s, "cancel", map[string]string{"id": id})
	mustCall(t, s, "cancel", map[string]string{"id": id})
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 1 || entries[0].Name() != "empty" || s.reserved != 0 {
		t.Fatalf("cancel leaked: %v %v", entries, err)
	}
}

func TestTraversalAndInvalidNames(t *testing.T) {
	s, _ := testSession(t)
	if _, err := relative(string([]byte{0xff})); err == nil {
		t.Fatal("invalid UTF-8 path accepted")
	}
	for _, name := range []string{"../escape", "a/../../escape", "/tmp/escape", "a/../b", "./b", "a//b", "a/", "a\\b", "C:/escape", "file:stream", "bad\x00name", "bad\nname", "bad*name", "end.", "end ", "NUL.txt", "COM1", tempPrefix + "mine", strings.Repeat("a", 256), strings.Repeat("a/", 600)} {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			for _, method := range []string{"list", "stat", "mkdir", "begin"} {
				in := map[string]any{"path": name}
				if method == "begin" {
					in["size"] = 0
				}
				if _, err := call(s, method, in); err == nil {
					t.Fatalf("%s accepted invalid path", method)
				}
			}
		})
	}
	for _, name := range []string{".", ""} {
		if _, err := call(s, "begin", map[string]any{"path": name, "size": 0}); err == nil {
			t.Fatal("allowed root as upload")
		}
		if _, err := call(s, "mkdir", map[string]string{"path": name}); err == nil {
			t.Fatal("allowed root mkdir")
		}
	}
}

func TestSymlinksRejectedAtAllLevels(t *testing.T) {
	s, home := testSession(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "real"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "real", "local"), []byte("local"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, link := range [][2]string{{"outside", outside}, {"inside", "real"}, {"file", "real/local"}} {
		if err := os.Symlink(link[1], filepath.Join(home, link[0])); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
	}
	for _, name := range []string{"outside", "inside", "outside/secret", "inside/local", "file"} {
		if _, err := call(s, "stat", map[string]string{"path": name}); err == nil {
			t.Fatalf("stat accepted symlink %s", name)
		}
	}
	for _, name := range []string{"outside", "inside", "file"} {
		if _, err := call(s, "list", map[string]string{"path": name}); err == nil {
			t.Fatalf("list accepted symlink %s", name)
		}
		if _, err := call(s, "begin", map[string]any{"path": name + "/new", "size": 1}); err == nil {
			t.Fatalf("write accepted symlink %s", name)
		}
		if _, err := call(s, "mkdir", map[string]string{"path": name + "/new"}); err == nil {
			t.Fatalf("mkdir accepted symlink %s", name)
		}
	}
	out := mustCall(t, s, "list", map[string]string{"path": "."}).(listResult)
	if len(out.Entries) != 1 || out.Entries[0].Name != "real" {
		t.Fatalf("list exposed links: %+v", out)
	}
}

func TestNoClobberAtBeginAndCommit(t *testing.T) {
	s, home := testSession(t)
	if err := os.WriteFile(filepath.Join(home, "exists"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "begin", map[string]any{"path": "exists", "size": 0}); err == nil {
		t.Fatal("begin accepted existing target")
	}
	id := startUpload(t, s, "race", 3)
	writeChunk(t, s, id, 0, []byte("new"))
	if err := os.WriteFile(filepath.Join(home, "race"), []byte("winner"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "commit", map[string]string{"id": id}); err == nil {
		t.Fatal("commit overwrote existing target")
	}
	got, _ := os.ReadFile(filepath.Join(home, "race"))
	if string(got) != "winner" {
		t.Fatal("target clobbered")
	}
	mustCall(t, s, "cancel", map[string]string{"id": id})
	id1 := startUpload(t, s, "compete", 1)
	id2 := startUpload(t, s, "compete", 1)
	writeChunk(t, s, id1, 0, []byte("1"))
	writeChunk(t, s, id2, 0, []byte("2"))
	mustCall(t, s, "commit", map[string]string{"id": id1})
	if _, err := call(s, "commit", map[string]string{"id": id2}); err == nil {
		t.Fatal("concurrent target overwritten")
	}
}

func TestUploadLimitsAndOffsets(t *testing.T) {
	s, _ := testSession(t)
	for _, size := range []int64{-1, MaxFileSize + 1, 1 << 53} {
		if _, err := call(s, "begin", map[string]any{"path": "bad", "size": size}); err == nil {
			t.Fatal("bad size accepted")
		}
	}
	id := startUpload(t, s, "huge", MaxFileSize)
	if _, err := call(s, "begin", map[string]any{"path": "over", "size": 1}); err == nil {
		t.Fatal("reservation cap exceeded")
	}
	mustCall(t, s, "cancel", map[string]string{"id": id})
	for i := 0; i < maxUploads; i++ {
		startUpload(t, s, fmt.Sprint("slot", i), 0)
	}
	if _, err := call(s, "begin", map[string]any{"path": "fifth", "size": 0}); err == nil {
		t.Fatal("upload slot cap exceeded")
	}
	s.mu.Lock()
	s.cancelAllLocked()
	s.mu.Unlock()
	id = startUpload(t, s, "small", 3)
	for _, in := range []map[string]any{
		{"id": id, "offset": -1, "data": "eA=="}, {"id": id, "offset": 1, "data": "eA=="},
		{"id": id, "offset": 0, "data": ""}, {"id": id, "offset": 0, "data": "!!!!"},
		{"id": id, "offset": 0, "data": base64.StdEncoding.EncodeToString(make([]byte, ChunkSize+1))},
		{"id": id, "offset": 0, "data": "eHh4eA=="},
	} {
		if _, err := call(s, "write", in); err == nil {
			t.Fatalf("bad write accepted: offset=%v", in["offset"])
		}
	}
	if _, err := call(s, "commit", map[string]string{"id": id}); err == nil {
		t.Fatal("incomplete commit accepted")
	}
	writeChunk(t, s, id, 0, []byte("abc"))
	if _, err := call(s, "write", map[string]any{"id": id, "offset": 0, "data": "YQ=="}); err == nil {
		t.Fatal("duplicate offset accepted")
	}
	mustCall(t, s, "commit", map[string]string{"id": id})
}

func TestReadVersionAndBounds(t *testing.T) {
	s, home := testSession(t)
	name := filepath.Join(home, "source")
	if err := os.WriteFile(name, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	info := mustCall(t, s, "stat", map[string]string{"path": "source"}).(Entry)
	base := map[string]any{"path": "source", "size": info.Size, "modified": info.Modified, "offset": 0, "length": 1}
	for _, change := range []map[string]any{{"offset": -1}, {"offset": info.Size + 1}, {"length": ChunkSize + 1}, {"length": -1}, {"size": info.Size + 1}, {"size": MaxFileSize + 1}, {"modified": ""}, {"modified": "bad"}} {
		in := map[string]any{}
		for k, v := range base {
			in[k] = v
		}
		for k, v := range change {
			in[k] = v
		}
		if _, err := call(s, "read", in); err == nil {
			t.Fatalf("bad read accepted: %v", change)
		}
	}
	if err := os.WriteFile(name, []byte("updated!"), 0600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(name, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "read", base); err == nil {
		t.Fatal("changed source accepted")
	}
}

func TestPaginationAndBoundedJSON(t *testing.T) {
	s, home := testSession(t)
	dir := strings.Repeat("長", 70)
	if err := os.Mkdir(filepath.Join(home, dir), 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 43; i++ {
		name := fmt.Sprintf("%03d-%s", i, strings.Repeat("文", 70))
		if err := os.WriteFile(filepath.Join(home, dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for offset := 0; offset != -1; {
		out := mustCall(t, s, "list", map[string]any{"path": dir, "offset": offset}).(listResult)
		if len(out.Entries) > pageSize {
			t.Fatal("page too large")
		}
		if out.NextOffset != -1 && out.NextOffset <= offset {
			t.Fatal("pagination did not advance")
		}
		for _, e := range out.Entries {
			if seen[e.Name] {
				t.Fatal("duplicate pagination entry")
			}
			seen[e.Name] = true
		}
		offset = out.NextOffset
	}
	if len(seen) != 43 {
		t.Fatalf("missing page entries: %d", len(seen))
	}
	for _, offset := range []int{-1, maxListOffset + 1} {
		if _, err := call(s, "list", map[string]any{"path": dir, "offset": offset}); err == nil {
			t.Fatal("bad list offset accepted")
		}
	}
	if _, err := call(s, "mkdir", map[string]string{"path": "missing/child"}); err == nil {
		t.Fatal("mkdir unexpectedly created parent")
	}
}

func TestExpirationAuthorizationAndCloseCleanup(t *testing.T) {
	s, home := testSession(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	id := startUpload(t, s, "expire", 50)
	now = now.Add(uploadIdle)
	if _, err := call(s, "write", map[string]any{"id": id, "offset": 0, "data": "YQ=="}); err == nil {
		t.Fatal("idle upload survived")
	}
	if len(s.uploads) != 0 || s.reserved != 0 {
		t.Fatal("expired upload reservation leaked")
	}
	id = startUpload(t, s, "unauthorized", 50)
	s.allowed = func() bool { return false }
	if _, err := call(s, "write", map[string]any{"id": id, "offset": 0, "data": "YQ=="}); err == nil {
		t.Fatal("authorization not rechecked")
	}
	if len(s.uploads) != 1 || s.reserved != 50 || s.uploads[id].offset != 0 {
		t.Fatal("authorization rejection must preserve staged data without writing")
	}
	s.allowed = func() bool { return true }
	startUpload(t, s, "disconnect", 100)
	s.close()
	s.close()
	files, err := os.ReadDir(home)
	if err != nil || len(files) != 0 {
		t.Fatalf("cleanup left files: %v %v", files, err)
	}
	if _, err := call(s, "list", map[string]string{"path": "."}); err == nil {
		t.Fatal("closed session remained usable")
	}
}

func TestMalformedCancelledAndPrivateErrors(t *testing.T) {
	s, home := testSession(t)
	for _, raw := range []string{"{", "{} {}", "null", "[]", " ", "{\"path\":\"\xff\"}", `{"path":".","unexpected":true}`, strings.Repeat(" ", 8193)} {
		if _, err := s.command(context.Background(), "list", []byte(raw)); err == nil {
			t.Fatal("bad JSON accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.command(ctx, "mkdir", []byte(`{"path":"cancelled"}`)); err == nil {
		t.Fatal("cancelled command executed")
	}
	_, err := call(s, "stat", map[string]string{"path": "private-missing"})
	if err == nil || strings.Contains(err.Error(), home) || strings.Contains(err.Error(), "private-missing") {
		t.Fatalf("private path leaked: %v", err)
	}
}

func TestParallelUploadCap(t *testing.T) {
	s, _ := testSession(t)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = call(s, "begin", map[string]any{"path": fmt.Sprintf("parallel-%d", i), "size": MaxFileSize / 4})
		}(i)
	}
	wg.Wait()
	if len(s.uploads) != 4 || s.reserved != MaxFileSize {
		t.Fatalf("limits raced: count=%d reserve=%d", len(s.uploads), s.reserved)
	}
}

func TestPaginationWorstCaseJSONEscaping(t *testing.T) {
	s, home := testSession(t)
	dir := strings.Repeat("&", 250) + "/" + strings.Repeat("&", 250) + "/" + strings.Repeat("&", 250)
	if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(dir)), 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("%d%s", i, strings.Repeat("&", 240))
		if err := s.root.WriteFile(dir+"/"+name, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	seen := 0
	for offset := 0; offset != -1; {
		out := mustCall(t, s, "list", map[string]any{"path": dir, "offset": offset}).(listResult)
		if out.NextOffset != -1 && out.NextOffset <= offset {
			t.Fatal("JSON-budget pagination stalled")
		}
		seen += len(out.Entries)
		offset = out.NextOffset
	}
	if seen != 5 {
		t.Fatalf("missing escaped names: %d", seen)
	}
}

func TestAtomicNoClobberConcurrentCreator(t *testing.T) {
	s, home := testSession(t)
	for i := 0; i < 32; i++ {
		name := fmt.Sprintf("atomic-%02d", i)
		id := startUpload(t, s, name, 3)
		writeChunk(t, s, id, 0, []byte("new"))
		start := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			<-start
			f, err := os.OpenFile(filepath.Join(home, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err == nil {
				_, err = f.Write([]byte("old"))
				f.Close()
			}
			result <- err
		}()
		close(start)
		_, commitErr := call(s, "commit", map[string]string{"id": id})
		createErr := <-result
		if (commitErr == nil) == (createErr == nil) {
			t.Fatalf("expected exactly one winner: commit=%v create=%v", commitErr, createErr)
		}
		got, err := os.ReadFile(filepath.Join(home, name))
		if err != nil {
			t.Fatal(err)
		}
		want := "new"
		if createErr == nil {
			want = "old"
		}
		if string(got) != want {
			t.Fatalf("clobbered creator: got %q want %q", got, want)
		}
		mustCall(t, s, "cancel", map[string]string{"id": id})
	}
}

func TestConcurrentFullChunkTransfers(t *testing.T) {
	s, home := testSession(t)
	data := bytes.Repeat([]byte{42, 255, 0, 128}, 64*1024)
	ids := make([]string, 4)
	for i := range ids {
		ids[i] = startUpload(t, s, fmt.Sprintf("bulk-%d", i), int64(len(data)))
	}
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			for offset := 0; offset < len(data); offset += ChunkSize {
				if _, err := call(s, "write", map[string]any{"id": id, "offset": offset, "data": base64.StdEncoding.EncodeToString(data[offset : offset+ChunkSize])}); err != nil {
					t.Errorf("bulk write: %v", err)
					return
				}
			}
			if _, err := call(s, "commit", map[string]string{"id": id}); err != nil {
				t.Errorf("bulk commit: %v", err)
				return
			}
			got, err := os.ReadFile(filepath.Join(home, fmt.Sprintf("bulk-%d", i)))
			if err != nil || !bytes.Equal(got, data) {
				t.Errorf("bulk content differs: %v", err)
			}
		}(i, id)
	}
	wg.Wait()
	if len(s.uploads) != 0 || s.reserved != 0 {
		t.Fatal("bulk transfer resources leaked")
	}
}
