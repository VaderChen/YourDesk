package filetransfer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func cancelState(t *testing.T, s *session, in any, want string) {
	t.Helper()
	out := mustCall(t, s, "cancel", in).(map[string]any)
	if out["ok"] != true || out["state"] != want {
		t.Fatalf("unexpected cancellation result: %v", out)
	}
}

func TestCancelDetachedUploadAfterFilesystemChanges(t *testing.T) {
	for _, change := range []string{"destination-conflict", "parent-moved", "stage-replaced", "stage-removed", "stage-modified"} {
		t.Run(change, func(t *testing.T) {
			hub, home, create := sharedHub(t)
			first := create(home)
			mustCall(t, first, "mkdir", map[string]string{"path": "folder"})
			id, token := resumableUpload(t, first, "folder/destination", MaxFileSize)
			writeChunk(t, first, id, 0, []byte("part"))
			stage := filepath.Join(home, "folder", hub.uploads[id].temp)
			first.detach()
			var protected string
			switch change {
			case "destination-conflict":
				protected = filepath.Join(home, "folder", "destination")
				if err := os.WriteFile(protected, []byte("unrelated"), 0600); err != nil {
					t.Fatal(err)
				}
			case "parent-moved":
				if err := os.Rename(filepath.Join(home, "folder"), filepath.Join(home, "moved")); err != nil {
					t.Fatal(err)
				}
				stage = filepath.Join(home, "moved", filepath.Base(stage))
			case "stage-replaced":
				if err := os.Rename(stage, filepath.Join(home, "external-rename")); err != nil {
					t.Fatal(err)
				}
				protected = stage
				if err := os.WriteFile(protected, []byte("unrelated"), 0600); err != nil {
					t.Fatal(err)
				}
			case "stage-removed":
				if err := os.Remove(stage); err != nil {
					t.Fatal(err)
				}
			case "stage-modified":
				if err := os.WriteFile(stage, []byte("longer than confirmed offset"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			second := create(home)
			args := resumeArgs(id, token, "folder/destination", MaxFileSize)
			if _, err := call(second, "resume", args); err == nil {
				t.Fatal("fixture unexpectedly remained resumable")
			}
			if _, err := call(second, "cancel", map[string]string{"id": id}); err == nil {
				t.Fatal("unclaimed ID-only cancel accepted")
			}
			cancelState(t, second, args, "cancelled")
			cancelState(t, second, args, "cancelled") // Lost cancellation reply.
			if len(hub.uploads) != 0 || hub.reserved != 0 {
				t.Fatal("cancellation left a detached reservation")
			}
			startUpload(t, second, "unrelated-new-upload", 1)
			if protected != "" {
				data, err := os.ReadFile(protected)
				if err != nil || string(data) != "unrelated" {
					t.Fatalf("cancellation changed unrelated file: %q %v", data, err)
				}
			}
			if change == "stage-replaced" {
				data, err := os.ReadFile(filepath.Join(home, "external-rename"))
				if err != nil || string(data) != "part" {
					t.Fatal("cancellation undid an external rename")
				}
			} else if _, err := os.Stat(stage); !os.IsNotExist(err) {
				t.Fatalf("owned stage was not removed: %v", err)
			}
		})
	}
}

func TestCancelCredentialsCannotCrossOwnershipOrHome(t *testing.T) {
	hub, home, create := sharedHub(t)
	first, second := create(home), create(home)
	id, token := resumableUpload(t, first, "guarded", 3)
	writeChunk(t, first, id, 0, []byte("abc"))
	args := resumeArgs(id, token, "guarded", 3)
	if _, err := call(second, "cancel", args); err == nil {
		t.Fatal("valid token stole another live owner's upload")
	}
	first.detach()
	otherHome := create(t.TempDir())
	if _, err := call(otherHome, "cancel", args); err == nil {
		t.Fatal("token cancelled a different home")
	}
	wrongToken := token[:32] + strings.Repeat("0", 32)
	if wrongToken == token {
		wrongToken = token[:32] + strings.Repeat("1", 32)
	}
	for _, bad := range []map[string]any{
		resumeArgs(id, wrongToken, "guarded", 3),
		resumeArgs(id, token, "other", 3),
		resumeArgs(id, token, "guarded", 2),
		resumeArgs(id, token, "guarded", -1),
		resumeArgs(id, token, "guarded", MaxFileSize+1),
		resumeArgs(id, "short", "guarded", 3),
		resumeArgs(id, strings.ToUpper(token), "guarded", 3),
		resumeArgs(id, token, ".", 3),
		resumeArgs(id, token, "../guarded", 3),
		{"id": id, "token": token, "path": "guarded"},
		{"id": id, "token": token, "size": 3},
		{"id": id, "path": "guarded", "size": 3},
	} {
		if _, err := call(second, "cancel", bad); err == nil {
			t.Fatal("invalid cancellation credentials accepted")
		}
		if hub.uploads[id] == nil || hub.uploads[id].owner != nil || hub.reserved != 3 {
			t.Fatal("invalid credentials mutated the retained upload")
		}
	}
	raw, _ := json.Marshal(args)
	ctx, stop := context.WithCancel(context.Background())
	stop()
	if _, err := second.command(ctx, "cancel", raw); err == nil {
		t.Fatal("cancelled request changed ownership")
	}
	second.allowed = func() bool { return false }
	if _, err := call(second, "cancel", args); err == nil {
		t.Fatal("unauthorized peer cancelled upload")
	}
	second.allowed = func() bool { return true }
	cancelState(t, second, args, "cancelled")
	if len(hub.uploads) != 0 || hub.reserved != 0 {
		t.Fatal("authorized token cancellation did not release quota")
	}
}

func TestCancelCompletionReceiptSurvivesLostReplyAndExpiry(t *testing.T) {
	hub, home, create := sharedHub(t)
	now := time.Now()
	hub.now = func() time.Time { return now }
	first := create(home)
	id, token := resumableUpload(t, first, "completed", 1)
	writeChunk(t, first, id, 0, []byte("x"))
	mustCall(t, first, "commit", map[string]string{"id": id})
	first.detach()
	if err := os.WriteFile(filepath.Join(home, "completed"), []byte("changed after commit"), 0600); err != nil {
		t.Fatal(err)
	}
	second := create(home)
	args := resumeArgs(id, token, "completed", 1)
	if _, err := call(second, "resume", args); err == nil {
		t.Fatal("changed completed file unexpectedly resumed")
	}
	cancelState(t, second, args, "complete")
	cancelState(t, second, args, "complete")                        // Lost cancellation response.
	cancelState(t, second, map[string]string{"id": id}, "complete") // Legacy owner.
	third := create(home)
	if _, err := call(third, "cancel", args); err == nil {
		t.Fatal("completion receipt stolen from live owner")
	}
	second.detach()
	cancelState(t, third, args, "complete")
	if len(hub.receipts) != 1 || len(hub.uploads) != 0 || hub.reserved != 0 {
		t.Fatal("completed cancellation lost receipt or leaked upload quota")
	}
	now = now.Add(uploadIdle)
	cancelState(t, third, args, "cancelled") // Expired receipt is a missing ID.
	if len(hub.receipts) != 0 {
		t.Fatal("cancellation refreshed the receipt TTL")
	}
	data, err := os.ReadFile(filepath.Join(home, "completed"))
	if err != nil || string(data) != "changed after commit" {
		t.Fatalf("completed cancellation/expiry changed user file: %q %v", data, err)
	}
}

func TestCancelLiveOwnerChecksProvidedTokenAndLegacyRemainsSupported(t *testing.T) {
	s, _ := testSession(t)
	id, token := resumableUpload(t, s, "owned", 0)
	wrong := token[:32] + strings.Repeat("0", 32)
	if wrong == token {
		wrong = token[:32] + strings.Repeat("1", 32)
	}
	if _, err := call(s, "cancel", resumeArgs(id, wrong, "owned", 0)); err == nil {
		t.Fatal("supplied bad token ignored for current owner")
	}
	cancelState(t, s, map[string]string{"id": id}, "cancelled")
	cancelState(t, s, map[string]string{"id": id}, "cancelled")
	id, token = resumableUpload(t, s, "ended-peer", 0)
	s.alive = func() bool { return false }
	second, err := newSessionWithHub(s.root.Name(), func() bool { return true }, s.transferHub)
	if err != nil {
		t.Fatal(err)
	}
	defer second.close()
	cancelState(t, second, resumeArgs(id, token, "ended-peer", 0), "cancelled")
	s.detach() // Late teardown must not undo successful cancellation.
	if s.reserved != 0 || len(s.uploads) != 0 {
		t.Fatal("peer-done cancellation retained quota")
	}
}
