package filetransfer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func sharedHub(t *testing.T) (*transferHub, string, func(string) *session) {
	t.Helper()
	home := t.TempDir()
	hub := newTransferHub()
	t.Cleanup(hub.close)
	create := func(home string) *session {
		s, err := newSessionWithHub(home, func() bool { return true }, hub)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.close)
		return s
	}
	return hub, home, create
}

func resumeArgs(id, token, name string, size int64) map[string]any {
	return map[string]any{"id": id, "token": token, "path": name, "size": size}
}

func resumableUpload(t *testing.T, s *session, name string, size int64) (string, string) {
	t.Helper()
	out := mustCall(t, s, "begin", map[string]any{"path": name, "size": size}).(map[string]any)
	id, token := out["id"].(string), out["resumeToken"].(string)
	if !validResumeToken(token) || id != token[:32] || out["state"] != "uploading" || out["nextOffset"] != int64(0) {
		t.Fatal("invalid begin contract")
	}
	return id, token
}

func TestResumeAcrossAuthorizedSessionsAndCommitAckLoss(t *testing.T) {
	hub, home, create := sharedHub(t)
	first := create(home)
	id, token := resumableUpload(t, first, "續傳.txt", 6)
	writeChunk(t, first, id, 0, []byte("abc"))
	first.detach()
	if len(hub.uploads) != 1 || hub.reserved != 6 || hub.uploads[id].owner != nil {
		t.Fatal("disconnect discarded upload")
	}
	second := create(home)
	for _, method := range []string{"write", "commit", "cancel"} {
		in := map[string]any{"id": id}
		if method == "write" {
			in["offset"] = 3
			in["data"] = "ZGVm"
		}
		if _, err := call(second, method, in); err == nil {
			t.Fatalf("%s accepted unclaimed upload", method)
		}
	}
	out := mustCall(t, second, "resume", resumeArgs(id, token, "續傳.txt", 6)).(map[string]any)
	if out["state"] != "uploading" || out["nextOffset"] != int64(3) {
		t.Fatalf("wrong resumed state: %v", out)
	}
	writeChunk(t, second, id, 3, []byte("def"))
	mustCall(t, second, "commit", map[string]string{"id": id})
	// Duplicate commit is safe in the current session, and a retained receipt
	// makes a lost commit reply distinguishable from an incomplete upload.
	mustCall(t, second, "commit", map[string]string{"id": id})
	second.detach()
	third := create(home)
	out = mustCall(t, third, "resume", resumeArgs(id, token, "續傳.txt", 6)).(map[string]any)
	if out["state"] != "complete" || out["nextOffset"] != int64(6) {
		t.Fatalf("missing completion receipt: %v", out)
	}
	mustCall(t, third, "cancel", map[string]string{"id": id})
	got, err := os.ReadFile(filepath.Join(home, "續傳.txt"))
	if err != nil || string(got) != "abcdef" {
		t.Fatalf("completed file changed after cancel: %q %v", got, err)
	}
	if len(hub.uploads) != 0 || hub.reserved != 0 || len(hub.receipts) != 1 {
		t.Fatal("retained accounting leaked")
	}
}

func TestResumeRejectsWrongTokenHomeMetadataAndLiveOwner(t *testing.T) {
	_, home, create := sharedHub(t)
	first, second := create(home), create(home)
	id, token := resumableUpload(t, first, "guarded", 8)
	if _, err := call(second, "resume", resumeArgs(id, token, "guarded", 8)); err == nil {
		t.Fatal("another live owner was stolen")
	}
	first.detach()
	wrong := token[:32] + strings.Repeat("0", 32)
	if wrong == token {
		wrong = token[:32] + strings.Repeat("1", 32)
	}
	for _, in := range []map[string]any{
		resumeArgs(id, wrong, "guarded", 8), resumeArgs(id, token, "different", 8), resumeArgs(id, token, "guarded", 9), resumeArgs(id, "short", "guarded", 8), resumeArgs(strings.Repeat("1", 32), token, "guarded", 8),
	} {
		if _, err := call(second, "resume", in); err == nil {
			t.Fatal("invalid resume accepted")
		}
	}
	otherHome := create(t.TempDir())
	if _, err := call(otherHome, "resume", resumeArgs(id, token, "guarded", 8)); err == nil {
		t.Fatal("token crossed home boundary")
	}
	if _, err := call(otherHome, "cancel", map[string]string{"id": id}); err == nil {
		t.Fatal("unclaimed cancellation accepted")
	}
	if first.uploads[id].owner != nil {
		t.Fatal("failed claims mutated owner")
	}
	mustCall(t, second, "resume", resumeArgs(id, token, "guarded", 8))
}

func TestBeginTokenIdempotencyAndLostReply(t *testing.T) {
	hub, home, create := sharedHub(t)
	first := create(home)
	token := strings.Repeat("a", 64)
	id := token[:32]
	in := map[string]any{"path": "first-ack-lost", "size": 4, "token": token}
	mustCall(t, first, "begin", in) // Client intentionally discards the response.
	writeChunk(t, first, id, 0, []byte("ab"))
	out := mustCall(t, first, "begin", in).(map[string]any)
	if out["nextOffset"] != int64(2) || out["id"] != id || len(hub.uploads) != 1 || hub.reserved != 4 {
		t.Fatal("duplicate begin reset or duplicated reservation")
	}
	for _, bad := range []map[string]any{
		{"path": "other", "size": 4, "token": token}, {"path": "first-ack-lost", "size": 5, "token": token}, {"path": "first-ack-lost", "size": 4, "token": token[:32] + strings.Repeat("b", 32)},
	} {
		if _, err := call(first, "begin", bad); err == nil {
			t.Fatal("mismatching begin reused token")
		}
	}
	first.detach()
	second := create(home)
	out = mustCall(t, second, "begin", in).(map[string]any)
	if out["nextOffset"] != int64(2) || out["state"] != "uploading" {
		t.Fatal("begin did not reclaim detached upload")
	}
	writeChunk(t, second, id, 2, []byte("cd"))
	mustCall(t, second, "commit", map[string]string{"id": id})
	out = mustCall(t, second, "begin", in).(map[string]any)
	if out["state"] != "complete" || out["nextOffset"] != int64(4) || len(hub.uploads) != 0 {
		t.Fatal("completed begin was not idempotent")
	}
	for _, badToken := range []string{strings.Repeat("A", 64), strings.Repeat("g", 64), "short", strings.Repeat("a", 65)} {
		if _, err := call(second, "begin", map[string]any{"path": "bad", "size": 0, "token": badToken}); err == nil {
			t.Fatal("invalid caller token accepted")
		}
	}
	// No receipt exists when the server never received begin: explicit cancel
	// is still idempotent, without inventing a second upload.
	mustCall(t, second, "cancel", map[string]string{"id": strings.Repeat("0", 32)})
}

func TestResumeHandlesPeerDoneBeforeDetachCallback(t *testing.T) {
	_, home, create := sharedHub(t)
	first, second := create(home), create(home)
	var alive atomic.Bool
	alive.Store(true)
	first.alive = alive.Load
	id, token := resumableUpload(t, first, "handoff", 1)
	alive.Store(false)
	mustCall(t, second, "resume", resumeArgs(id, token, "handoff", 1))
	first.detach() // Late old-peer teardown must not detach the new owner.
	if second.uploads[id].owner != second {
		t.Fatal("old peer detached replacement owner")
	}
	writeChunk(t, second, id, 0, []byte("x"))
	mustCall(t, second, "commit", map[string]string{"id": id})
}

func TestAuthorizationLossDuringDisconnectRetainsResume(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		_, home, create := sharedHub(t)
		first := create(home)
		var authorized atomic.Bool
		authorized.Store(true)
		first.allowed = authorized.Load
		id, token := resumableUpload(t, first, "disconnect", 3)
		writeChunk(t, first, id, 0, []byte("abc"))
		authorized.Store(false)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); first.detach() }()
		go func() {
			defer wg.Done()
			if _, err := call(first, "write", map[string]any{"id": id, "offset": 3, "data": "ZA=="}); err == nil {
				t.Error("unauthorized write accepted")
			}
		}()
		wg.Wait()
		second := create(home)
		out := mustCall(t, second, "resume", resumeArgs(id, token, "disconnect", 3)).(map[string]any)
		if out["nextOffset"] != int64(3) {
			t.Fatal("authorization teardown discarded confirmed bytes")
		}
		mustCall(t, second, "cancel", map[string]string{"id": id})
	}
}

func TestGlobalRetainedLimitsAndExpiry(t *testing.T) {
	hub, home, create := sharedHub(t)
	now := time.Now()
	hub.now = func() time.Time { return now }
	first := create(home)
	ids, tokens := make([]string, 4), make([]string, 4)
	for i := range ids {
		ids[i], tokens[i] = resumableUpload(t, first, fmt.Sprintf("retained-%d", i), MaxFileSize/4)
	}
	first.detach()
	second := create(home)
	if _, err := call(second, "begin", map[string]any{"path": "fifth", "size": 0}); err == nil {
		t.Fatal("detached reservations evaded global slots")
	}
	otherHome := create(t.TempDir())
	if _, err := call(otherHome, "begin", map[string]any{"path": "other-home", "size": 1}); err == nil {
		t.Fatal("home switch evaded global bytes")
	}
	mustCall(t, second, "resume", resumeArgs(ids[0], tokens[0], "retained-0", MaxFileSize/4))
	mustCall(t, second, "cancel", map[string]string{"id": ids[0]})
	startUpload(t, otherHome, "other-home", 1)
	if hub.reserved != 3*MaxFileSize/4+1 {
		t.Fatal("cancel reservation accounting wrong")
	}
	now = now.Add(uploadIdle)
	hub.mu.Lock()
	hub.expireLocked()
	hub.mu.Unlock()
	if len(hub.uploads) != 0 || hub.reserved != 0 {
		t.Fatal("24-hour cleanup retained staged resources")
	}
	files, err := os.ReadDir(home)
	if err != nil || len(files) != 0 {
		t.Fatalf("expired staging files remain: %v", err)
	}
	if _, err := call(second, "resume", resumeArgs(ids[1], tokens[1], "retained-1", MaxFileSize/4)); err == nil {
		t.Fatal("expired upload resumed")
	}
}

func TestCompletedReceiptBoundTTLAndModifiedFile(t *testing.T) {
	hub, home, create := sharedHub(t)
	now := time.Now()
	hub.now = func() time.Time { return now }
	s := create(home)
	var oldest, lastID, lastToken string
	for i := 0; i < maxReceipts+3; i++ {
		id, token := resumableUpload(t, s, fmt.Sprintf("done-%d", i), 0)
		if i == 0 {
			oldest = id
		}
		mustCall(t, s, "commit", map[string]string{"id": id})
		lastID, lastToken = id, token
		now = now.Add(time.Second)
	}
	if len(hub.receipts) != maxReceipts || hub.receipts[oldest] != nil {
		t.Fatal("receipt cap did not evict oldest")
	}
	lastName := fmt.Sprintf("done-%d", maxReceipts+2)
	if err := os.WriteFile(filepath.Join(home, lastName), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "resume", resumeArgs(lastID, lastToken, lastName, 0)); err == nil {
		t.Fatal("modified completed file reported intact")
	}
	now = now.Add(uploadIdle)
	hub.mu.Lock()
	hub.expireLocked()
	hub.mu.Unlock()
	if len(hub.receipts) != 0 {
		t.Fatal("completed receipts did not expire")
	}
	if info, err := os.Stat(filepath.Join(home, lastName)); err != nil || info.Size() != 7 {
		t.Fatal("receipt expiry touched committed file")
	}
}

func TestResumeRejectsCancelledUnauthorizedAndReplacedStaging(t *testing.T) {
	_, home, create := sharedHub(t)
	first := create(home)
	id, token := resumableUpload(t, first, "tampered", 3)
	writeChunk(t, first, id, 0, []byte("abc"))
	stage := first.uploads[id].temp
	first.detach()
	second := create(home)
	raw, _ := json.Marshal(resumeArgs(id, token, "tampered", 3))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := second.command(ctx, "resume", raw); err == nil {
		t.Fatal("cancelled resume claimed upload")
	}
	second.allowed = func() bool { return false }
	if _, err := call(second, "resume", resumeArgs(id, token, "tampered", 3)); err == nil {
		t.Fatal("unauthorized resume claimed upload")
	}
	second.allowed = func() bool { return true }
	if err := os.Remove(filepath.Join(home, stage)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, stage), []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := call(second, "resume", resumeArgs(id, token, "tampered", 3)); err == nil {
		t.Fatal("replaced staged inode accepted")
	}
	if second.uploads[id].owner != nil {
		t.Fatal("failed resume claimed owner")
	}
}

func TestCancelAndExpiryPreserveReplacementStaging(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(fmt.Sprintf("expire-%t", expire), func(t *testing.T) {
			hub, home, create := sharedHub(t)
			now := time.Now()
			hub.now = func() time.Time { return now }
			s := create(home)
			id, _ := resumableUpload(t, s, "target", 1)
			stage := hub.uploads[id].temp
			if err := os.Rename(filepath.Join(home, stage), filepath.Join(home, "moved-original")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, stage), []byte("unrelated"), 0600); err != nil {
				t.Fatal(err)
			}
			if expire {
				s.detach()
				now = now.Add(uploadIdle)
				hub.mu.Lock()
				hub.expireLocked()
				hub.mu.Unlock()
			} else {
				mustCall(t, s, "cancel", map[string]string{"id": id})
			}
			got, err := os.ReadFile(filepath.Join(home, stage))
			if err != nil || string(got) != "unrelated" {
				t.Fatalf("replacement was removed: %q %v", got, err)
			}
			if _, err := os.Stat(filepath.Join(home, "moved-original")); err != nil {
				t.Fatal("external rename was undone")
			}
			if len(hub.uploads) != 0 || hub.reserved != 0 {
				t.Fatal("replacement prevented resource release")
			}
		})
	}
}

func TestCommitRejectsReplacedDestinationParent(t *testing.T) {
	_, home, create := sharedHub(t)
	s := create(home)
	mustCall(t, s, "mkdir", map[string]string{"path": "folder"})
	id, _ := resumableUpload(t, s, "folder/file", 1)
	writeChunk(t, s, id, 0, []byte("x"))
	if err := os.Rename(filepath.Join(home, "folder"), filepath.Join(home, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "commit", map[string]string{"id": id}); err == nil {
		t.Fatal("committed into moved parent")
	}
	for _, name := range []string{"folder/file", "moved/file"} {
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Fatal("unexpected committed target")
		}
	}
	mustCall(t, s, "cancel", map[string]string{"id": id})
}
