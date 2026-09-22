package serviceupdate

import (
	"errors"
	"reflect"
	"testing"
)

type fakeService struct {
	calls  []string
	fail   string
	failed bool
}

func (f *fakeService) call(name string) error {
	f.calls = append(f.calls, name)
	if name == f.fail && !f.failed {
		f.failed = true
		return errors.New("fixture " + name)
	}
	return nil
}
func (f *fakeService) Stop() error    { return f.call("stop") }
func (f *fakeService) Backup() error  { return f.call("backup") }
func (f *fakeService) Install() error { return f.call("install") }
func (f *fakeService) Start() error   { return f.call("start") }
func (f *fakeService) Verify(updated bool) error {
	if updated {
		return f.call("verify-new")
	}
	return f.call("verify-old")
}
func (f *fakeService) Restore() error { return f.call("restore") }

func TestServiceUpdateSuccessfulAndIdempotent(t *testing.T) {
	f := &fakeService{}
	job := NewTransaction(f)
	for _, op := range []string{"begin", "begin", "commit", "commit", "finish", "finish"} {
		if r := job.Command(op); r.Error != "" {
			t.Fatal(r)
		}
	}
	if job.Phase != "complete" || !reflect.DeepEqual(f.calls, []string{"stop", "backup", "install", "start", "verify-new"}) {
		t.Fatal(job.Phase, f.calls)
	}
	if r := job.Command("rollback"); r.Error == "" {
		t.Fatal("已確認更新不應被舊訊息回復")
	}
}

func TestServiceUpdateFailureRestoresBeforeReporting(t *testing.T) {
	for _, failure := range []string{"stop", "backup", "install", "start", "verify-new"} {
		t.Run(failure, func(t *testing.T) {
			f := &fakeService{fail: failure}
			job := NewTransaction(f)
			r := job.Command("begin")
			if r.Error == "" {
				r = job.Command("commit")
			}
			if r.Error == "" || r.Phase != "rolled-back" {
				t.Fatal(r, f.calls)
			}
			want := []string{"start", "verify-old"}
			if failure != "stop" && failure != "backup" {
				want = []string{"stop", "restore", "start", "verify-old"}
			}
			if len(f.calls) < len(want) || !reflect.DeepEqual(f.calls[len(f.calls)-len(want):], want) {
				t.Fatal("沒有完整回復", f.calls)
			}
		})
	}
}

func TestAbandonedUpdateAndOutOfOrderCommands(t *testing.T) {
	for _, phase := range []string{"ready", "stopped", "applied"} {
		t.Run(phase, func(t *testing.T) {
			f := &fakeService{}
			job := NewTransaction(f)
			if phase != "ready" {
				job.Command("begin")
			}
			if phase == "applied" {
				job.Command("commit")
			}
			if err := job.Rollback(); err != nil {
				t.Fatal(err)
			}
			if job.Phase != "rolled-back" {
				t.Fatal(job.Phase)
			}
			if phase == "ready" && len(f.calls) != 0 {
				t.Fatal("尚未停止的服務不應被改動")
			}
		})
	}
	f := &fakeService{}
	job := NewTransaction(f)
	if r := job.Command("commit"); r.Error == "" || len(f.calls) != 0 {
		t.Fatal(r, f.calls)
	}
	job.Command("begin")
	job.Command("commit")
	before := len(f.calls)
	if r := job.Command("begin"); r.Error == "" || len(f.calls) != before || job.Phase != "applied" {
		t.Fatal(r, f.calls)
	}
	f.fail = "restore"
	if err := job.Rollback(); err == nil || job.Phase != "failed" {
		t.Fatal("回復失敗應保留工作與備份")
	}
	if err := job.Rollback(); err != nil || job.Phase != "rolled-back" {
		t.Fatal("應允許再次回復", err)
	}
}
