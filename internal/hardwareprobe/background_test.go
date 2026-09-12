package hardwareprobe

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if HandleHelper(os.Args[1:]) {
		return
	}
	os.Exit(m.Run())
}
func TestBackgroundDetectorSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(3*((len(platformJobs())+Workers-1)/Workers)*8+10)*time.Second)
	defer cancel()
	Start(ctx)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("背景偵測未在 smoke 預算內結束")
	}
	s := Snapshot()
	if s.Workers != 4 || s.Completed != len(platformJobs()) || s.Status != "complete" {
		t.Fatalf("偵測未完整結束：%+v", s)
	}
	// 連續兩輪驗證完整重測、禁止重入，以及保留原始啟動結果。
	for run := 0; run < 2; run++ {
		if !StartDeep() {
			t.Fatal("無法啟動深度測試")
		}
		if StartDeep() {
			t.Fatal("深度測試不應允許重入")
		}
		mu.RLock()
		finished := deepDone
		mu.RUnlock()
		select {
		case <-finished:
		case <-ctx.Done():
			t.Fatal("深度測試未在 smoke 預算內結束")
		}
		deep := DeepSnapshot()
		if deep.Status != "complete" || deep.Completed != len(platformJobs()) || len(deep.Results) != len(platformJobs()) {
			t.Fatalf("深度測試未完整結束：%+v", deep)
		}
		for i, j := range platformJobs() {
			if deep.Results[i].Key != key(j) || deep.Results[i].State != "complete" {
				t.Fatalf("項目未完整執行：%+v", deep.Results[i])
			}
		}
		if !reflect.DeepEqual(s, Snapshot()) {
			t.Fatal("深度測試改寫了啟動結果")
		}
	}
	p := Policy()
	if p == nil {
		t.Fatal("偵測結果未產生策略")
	}
	t.Logf("%d 項偵測，4 路，%.1f ms，策略 %d 個尺寸能力，快照預算 %d", s.Completed, s.DurationMS, len(p.Encoders), p.CompareBytes)
}
