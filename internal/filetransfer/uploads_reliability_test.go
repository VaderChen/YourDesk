package filetransfer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func denyStageRemoval(t *testing.T, directory string) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix directory permissions and an unprivileged account")
	}
	if err := os.Chmod(directory, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0700) })
}

func TestCancelRetriesAfterStageRemovalFailure(t *testing.T) {
	hub, home, create := sharedHub(t)
	first := create(home)
	mustCall(t, first, "mkdir", map[string]string{"path": "folder"})
	id, token := resumableUpload(t, first, "folder/upload", MaxFileSize)
	writeChunk(t, first, id, 0, []byte("part"))
	stage := filepath.Join(home, "folder", hub.uploads[id].temp)
	args := resumeArgs(id, token, "folder/upload", MaxFileSize)
	denyStageRemoval(t, filepath.Join(home, "folder"))
	if _, err := call(first, "cancel", args); err == nil {
		t.Fatal("cancel reported success although stage removal was denied")
	}
	if hub.uploads[id] == nil || hub.reserved != MaxFileSize {
		t.Fatal("failed cancellation discarded credentials or quota")
	}
	first.detach()
	second := create(home)
	if _, err := call(second, "cancel", args); err == nil {
		t.Fatal("reconnected cancellation hid the removal failure")
	}
	if err := os.Chmod(filepath.Join(home, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	cancelState(t, second, args, "cancelled")
	cancelState(t, second, args, "cancelled")
	if hub.uploads[id] != nil || hub.reserved != 0 {
		t.Fatal("successful retry did not release quota")
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("successful retry left the stage: %v", err)
	}
}

func TestExpiryRetriesAfterStageRemovalFailure(t *testing.T) {
	hub, home, create := sharedHub(t)
	now := time.Now()
	hub.now = func() time.Time { return now }
	s := create(home)
	mustCall(t, s, "mkdir", map[string]string{"path": "folder"})
	id, _ := resumableUpload(t, s, "folder/upload", 3)
	stage := filepath.Join(home, "folder", hub.uploads[id].temp)
	denyStageRemoval(t, filepath.Join(home, "folder"))
	s.detach()
	now = now.Add(uploadIdle)
	hub.expireLocked()
	if hub.uploads[id] == nil || hub.reserved != 3 {
		t.Fatal("failed expiry cleanup discarded its retry state")
	}
	if err := os.Chmod(filepath.Join(home, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	hub.expireLocked()
	if hub.uploads[id] != nil || hub.reserved != 0 {
		t.Fatal("expiry retry did not release quota")
	}
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Fatalf("expiry retry left the stage: %v", err)
	}
}

func TestCancelRetainsUploadWhenStageLookupFails(t *testing.T) {
	s, home := testSession(t)
	id := startUpload(t, s, "upload", 3)
	u := s.uploads[id]
	// Closing this test-owned root deterministically injects an Lstat error.
	if err := u.parent.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "cancel", map[string]string{"id": id}); err == nil {
		t.Fatal("cancel reported success without confirming stage ownership")
	}
	if s.uploads[id] != u || s.reserved != 3 {
		t.Fatal("failed lookup discarded the upload")
	}
	reopened, err := os.OpenRoot(home)
	if err != nil {
		t.Fatal(err)
	}
	u.parent = reopened
	cancelState(t, s, map[string]string{"id": id}, "cancelled")
	if _, err := os.Stat(filepath.Join(home, u.temp)); !os.IsNotExist(err) {
		t.Fatalf("retry left the stage: %v", err)
	}
}

func TestCancelAfterStageHandleClosed(t *testing.T) {
	s, home := testSession(t)
	id, token := resumableUpload(t, s, "upload", 3)
	u := s.uploads[id]
	if err := u.file.Close(); err != nil {
		t.Fatal(err)
	}
	cancelState(t, s, resumeArgs(id, token, "upload", 3), "cancelled")
	if s.reserved != 0 || s.uploads[id] != nil {
		t.Fatal("closed stage handle stranded the reservation")
	}
	if _, err := os.Stat(filepath.Join(home, u.temp)); !os.IsNotExist(err) {
		t.Fatalf("closed stage handle stranded the temporary file: %v", err)
	}
}

func TestWriteFailureRetainsUnconfirmedCleanup(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(fmt.Sprintf("closed=%t", closed), func(t *testing.T) {
			hub, home, create := sharedHub(t)
			s := create(home)
			mustCall(t, s, "mkdir", map[string]string{"path": "folder"})
			id, token := resumableUpload(t, s, "folder/upload", 6)
			writeChunk(t, s, id, 0, []byte("abc"))
			u := hub.uploads[id]
			// The same inode opened read-only gives a deterministic OS write failure.
			if err := u.file.Close(); err != nil {
				t.Fatal(err)
			}
			if !closed {
				readOnly, err := u.parent.Open(u.temp)
				if err != nil {
					t.Fatal(err)
				}
				u.file = readOnly
			}
			denyStageRemoval(t, filepath.Join(home, "folder"))
			if _, err := call(s, "write", map[string]any{"id": id, "offset": 3, "data": "ZGVm"}); err == nil {
				t.Fatal("read-only stage accepted a write")
			}
			if hub.uploads[id] != u || hub.reserved != 6 || u.offset != 3 {
				t.Fatal("write failure lost the pending cleanup or confirmed offset")
			}
			if err := os.Chmod(filepath.Join(home, "folder"), 0700); err != nil {
				t.Fatal(err)
			}
			cancelState(t, s, resumeArgs(id, token, "folder/upload", 6), "cancelled")
			if hub.reserved != 0 {
				t.Fatal("write-failure cleanup retained quota")
			}
		})
	}
}

func TestUploadDecodeBufferBoundary(t *testing.T) {
	s, home := testSession(t)
	id := startUpload(t, s, "upload", ChunkSize)
	for _, data := range []string{
		base64.StdEncoding.EncodeToString(make([]byte, ChunkSize+1)),
		// The largest admitted encoded length can decode to ChunkSize+2 bytes.
		strings.Repeat("A", base64.StdEncoding.EncodedLen(ChunkSize)),
		strings.Repeat("A", base64.StdEncoding.EncodedLen(ChunkSize)-1) + "!",
	} {
		if _, err := call(s, "write", map[string]any{"id": id, "offset": 0, "data": data}); err == nil {
			t.Fatal("invalid or oversized decoded chunk accepted")
		}
		if s.uploads[id].offset != 0 {
			t.Fatal("rejected chunk advanced the offset")
		}
	}
	want := bytes.Repeat([]byte{0x73}, ChunkSize)
	writeChunk(t, s, id, 0, want)
	mustCall(t, s, "commit", map[string]string{"id": id})
	got, err := os.ReadFile(filepath.Join(home, "upload"))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("rejected input polluted subsequent chunk: %v", err)
	}
}

func TestUploadWriteBufferIsolationAcrossSessions(t *testing.T) {
	_, home, create := sharedHub(t)
	var wg sync.WaitGroup
	for i := 0; i < maxUploads; i++ {
		s := create(home)
		name := fmt.Sprintf("upload-%d", i)
		want := bytes.Repeat([]byte{byte(i + 1)}, 2*ChunkSize+17+i)
		id := startUpload(t, s, name, int64(len(want)))
		wg.Add(1)
		go func() {
			defer wg.Done()
			for offset := 0; offset < len(want); offset += ChunkSize {
				data := base64.StdEncoding.EncodeToString(want[offset:min(offset+ChunkSize, len(want))])
				if _, err := call(s, "write", map[string]any{"id": id, "offset": offset, "data": data}); err != nil {
					t.Errorf("write %s: %v", name, err)
					return
				}
			}
			if _, err := call(s, "commit", map[string]string{"id": id}); err != nil {
				t.Errorf("commit %s: %v", name, err)
				return
			}
			got, err := os.ReadFile(filepath.Join(home, name))
			if err != nil || !bytes.Equal(got, want) {
				t.Errorf("chunk buffer crossed sessions for %s: %v", name, err)
			}
		}()
	}
	wg.Wait()
}

func BenchmarkUploadWriteChunk(b *testing.B) {
	s, err := newSession(b.TempDir(), func() bool { return true })
	if err != nil {
		b.Fatal(err)
	}
	defer s.close()
	begin, err := call(s, "begin", map[string]any{"path": "upload", "size": ChunkSize})
	if err != nil {
		b.Fatal(err)
	}
	id := begin.(map[string]any)["id"].(string)
	u := s.uploads[id]
	data := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x7f}, ChunkSize))
	raw, err := json.Marshal(map[string]any{"id": id, "offset": 0, "data": data})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(ChunkSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.command(context.Background(), "write", raw); err != nil {
			b.Fatal(err)
		}
		// Reuse one real stage so benchmark duration cannot grow the disk usage.
		u.offset = 0
		if _, err := u.file.Seek(0, io.SeekStart); err != nil {
			b.Fatal(err)
		}
	}
}
