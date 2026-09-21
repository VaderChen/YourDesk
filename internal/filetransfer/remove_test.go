package filetransfer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func removeArgs(t *testing.T, s *session, name string, recursive bool) map[string]any {
	t.Helper()
	info := mustCall(t, s, "stat", map[string]string{"path": name}).(Entry)
	return map[string]any{"path": name, "directory": info.Directory, "modified": info.Modified, "size": info.Size, "confirm": path.Base(name), "recursive": recursive}
}

func TestRemoveFileEmptyDirectoryAndRecursiveTree(t *testing.T) {
	s, home := testSession(t)
	if err := os.WriteFile(filepath.Join(home, "中文.txt"), []byte("remove me"), 0600); err != nil {
		t.Fatal(err)
	}
	out := mustCall(t, s, "remove", removeArgs(t, s, "中文.txt", false)).(removeResult)
	if !out.OK || out.RemovedCount != 1 || out.Partial {
		t.Fatalf("bad file remove result: %+v", out)
	}
	mustCall(t, s, "mkdir", map[string]string{"path": "empty"})
	out = mustCall(t, s, "remove", removeArgs(t, s, "empty", false)).(removeResult)
	if !out.OK || out.RemovedCount != 1 {
		t.Fatal("empty directory failed")
	}
	if err := os.MkdirAll(filepath.Join(home, "tree", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "tree", "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "tree", "nested", "file"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "untouched"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "remove", removeArgs(t, s, "tree", false)); err == nil {
		t.Fatal("nonempty directory removed without recursive consent")
	}
	out = mustCall(t, s, "remove", removeArgs(t, s, "tree", true)).(removeResult)
	if !out.OK || out.RemovedCount != 4 {
		t.Fatalf("bad recursive removal count: %+v", out)
	}
	files, err := os.ReadDir(home)
	if err != nil || len(files) != 1 || files[0].Name() != "untouched" {
		t.Fatalf("removed outside target: %v %v", files, err)
	}
}

func TestRemoveRequiresConfirmationVersionAndSafePath(t *testing.T) {
	s, home := testSession(t)
	if err := os.WriteFile(filepath.Join(home, "target"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	base := removeArgs(t, s, "target", false)
	for _, change := range []map[string]any{
		{"confirm": ""}, {"confirm": "wrong"}, {"modified": ""}, {"modified": "stale"}, {"size": -1}, {"size": 5}, {"directory": true}, {"recursive": true},
		{"path": "../target", "confirm": "target"}, {"path": "/target", "confirm": "target"}, {"path": ".", "confirm": "."}, {"path": "", "confirm": ""},
		{"path": strings.ToUpper(tempPrefix) + "reserved", "confirm": strings.ToUpper(tempPrefix) + "reserved"},
	} {
		in := map[string]any{}
		for k, v := range base {
			in[k] = v
		}
		for k, v := range change {
			in[k] = v
		}
		if _, err := call(s, "remove", in); err == nil {
			t.Fatalf("unsafe remove accepted: %v", change)
		}
		if _, err := os.Stat(filepath.Join(home, "target")); err != nil {
			t.Fatal("invalid confirmation deleted target")
		}
	}
	if err := os.WriteFile(filepath.Join(home, "target"), []byte("changed-size"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "remove", base); err == nil {
		t.Fatal("stale file version accepted")
	}
}

func TestRemovePreflightRejectsLinksWithoutPartialDeletion(t *testing.T) {
	s, home := testSession(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "tree"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "tree", "safe"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "tree", "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := call(s, "remove", removeArgs(t, s, "tree", true)); err == nil {
		t.Fatal("recursive removal traversed a link")
	}
	if _, err := os.Stat(filepath.Join(home, "tree", "safe")); err != nil {
		t.Fatal("preflight failure partially removed tree")
	}
	if _, err := os.Stat(filepath.Join(outside, "secret")); err != nil {
		t.Fatal("symlink escaped removal scope")
	}
	link, err := os.Lstat(filepath.Join(home, "tree", "link"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(s, "remove", map[string]any{"path": "tree/link", "confirm": "link", "directory": false, "size": link.Size(), "modified": link.ModTime().UTC().Format(time.RFC3339Nano), "recursive": false}); err == nil {
		t.Fatal("direct symlink removal accepted")
	}
}

func TestRemoveProtectsRetainedUploadsAndReservedTemps(t *testing.T) {
	_, home, create := sharedHub(t)
	first := create(home)
	mustCall(t, first, "mkdir", map[string]string{"path": "folder"})
	id, token := resumableUpload(t, first, "folder/file", 1)
	first.detach()
	second := create(home)
	if _, err := call(second, "remove", removeArgs(t, second, "folder", true)); err == nil {
		t.Fatal("removed retained upload subtree")
	}
	stage := second.uploads[id].temp
	stageInfo, err := os.Stat(filepath.Join(home, "folder", stage))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(second, "remove", map[string]any{"path": "folder/" + strings.ToUpper(stage), "confirm": strings.ToUpper(stage), "directory": false, "size": 0, "modified": stageInfo.ModTime().UTC().Format(time.RFC3339Nano), "recursive": false}); err == nil {
		t.Fatal("case alias bypassed protected temporary prefix")
	}
	mustCall(t, second, "resume", resumeArgs(id, token, "folder/file", 1))
	mustCall(t, second, "cancel", map[string]string{"id": id})
	if err := os.WriteFile(filepath.Join(home, "folder", tempPrefix+"orphan.part"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := call(second, "remove", removeArgs(t, second, "folder", true)); err == nil {
		t.Fatal("removed protected orphan staging name")
	}
}

func TestRemovePreflightEntryAndDepthLimits(t *testing.T) {
	t.Run("entries", func(t *testing.T) {
		s, home := testSession(t)
		if err := os.Mkdir(filepath.Join(home, "large"), 0700); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < maxRemoveEntries; i++ {
			if err := os.WriteFile(filepath.Join(home, "large", fmt.Sprintf("%04d", i)), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := call(s, "remove", removeArgs(t, s, "large", true)); err == nil {
			t.Fatal("oversized remove tree accepted")
		}
		files, err := os.ReadDir(filepath.Join(home, "large"))
		if err != nil || len(files) != maxRemoveEntries {
			t.Fatal("oversized preflight deleted files")
		}
	})
	t.Run("depth", func(t *testing.T) {
		s, home := testSession(t)
		deep := strings.TrimSuffix(strings.Repeat("a/", maxRemoveDepth+1), "/")
		if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(deep)), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := call(s, "remove", removeArgs(t, s, "a", true)); err == nil {
			t.Fatal("too-deep remove tree accepted")
		}
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(deep))); err != nil {
			t.Fatal("deep preflight partially deleted tree")
		}
	})
}

// This context deterministically cancels immediately after the first child
// disappears, instead of relying on scheduler timing or touching real files.
type cancelAfterChildRemoved struct {
	context.Context
	cancel    context.CancelFunc
	directory string
	initial   int
}

func (c *cancelAfterChildRemoved) Err() error {
	entries, err := os.ReadDir(c.directory)
	if err == nil && len(entries) < c.initial {
		c.cancel()
	}
	return c.Context.Err()
}

func TestRemovePartialCancellationIsStructured(t *testing.T) {
	s, home := testSession(t)
	dir := filepath.Join(home, "tree")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(removeArgs(t, s, "tree", true))
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelAfterChildRemoved{Context: base, cancel: cancel, directory: dir, initial: 2}
	result, err := s.command(ctx, "remove", raw)
	if err != nil {
		t.Fatalf("partial count was hidden by transport error: %v", err)
	}
	out := result.(removeResult)
	if out.OK || !out.Partial || out.RemovedCount != 1 || out.Error == "" || strings.Contains(out.Error, home) {
		t.Fatalf("bad partial response: %+v", out)
	}
	remaining, err := os.ReadDir(dir)
	if err != nil || len(remaining) != 1 {
		t.Fatalf("wrong remaining tree: %v %v", remaining, err)
	}
}

func TestRemoveCancelledOrUnauthorizedNeverMutates(t *testing.T) {
	s, home := testSession(t)
	if err := os.WriteFile(filepath.Join(home, "keep"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	in := removeArgs(t, s, "keep", false)
	raw, _ := json.Marshal(in)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.command(ctx, "remove", raw); err == nil {
		t.Fatal("cancelled deletion ran")
	}
	s.allowed = func() bool { return false }
	if _, err := call(s, "remove", in); err == nil {
		t.Fatal("unauthorized deletion ran")
	}
	if _, err := os.Stat(filepath.Join(home, "keep")); err != nil {
		t.Fatal("rejected deletion changed file")
	}
}

func TestRemoveValidationRejectsReplacedChild(t *testing.T) {
	s, home := testSession(t)
	if err := os.Mkdir(filepath.Join(home, "tree"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "tree", "original"), []byte("abc"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := s.root.Lstat("tree")
	if err != nil {
		t.Fatal(err)
	}
	node := &removeNode{name: "tree", info: info}
	count := 0
	if err := s.preflightRemove(context.Background(), s.root, node, "tree", 1, &count, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(home, "tree", "original"), filepath.Join(home, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "tree", "original"), []byte("xyz"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateRemove(context.Background(), s.root, node); err == nil {
		t.Fatal("replacement accepted after preflight")
	}
	got, err := os.ReadFile(filepath.Join(home, "tree", "original"))
	if err != nil || string(got) != "xyz" {
		t.Fatal("replacement changed despite validation failure")
	}
}
