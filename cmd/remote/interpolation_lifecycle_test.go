package main

import (
	"context"
	"image"
	"testing"
	"time"
)

func TestInterpolationResetDetachesLateResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	previousResults := make(chan interpolationResult, 1)
	frame := image.NewRGBA(image.Rect(0, 0, 16, 16))
	p := frameInterpolator{results: previousResults, cancel: cancel, previous: frame, shown: frame, target: frame, queued: frame, generated: true, synthesizing: true, deadline: time.Now(), period: time.Second, queuedAt: time.Now()}
	// 模擬已完成但尚未被顯示工作讀取的影像。
	previousResults <- interpolationResult{frame: frame, generation: p.generation}
	p.reset()
	if ctx.Err() == nil || p.results != nil || p.previous != nil || p.shown != nil || p.target != nil || p.queued != nil || p.generated || p.synthesizing || !p.deadline.IsZero() || p.period != 0 || !p.queuedAt.IsZero() {
		t.Fatal("重設仍持有舊影格或排程狀態")
	}
	<-previousResults
	// 原生呼叫較晚返回時，只能寫回其原通道。
	previousResults <- interpolationResult{frame: frame, generation: 0}
	p.results = make(chan interpolationResult, 1)
	if len(p.results) != 0 {
		t.Fatal("新工作階段收到舊影像")
	}
}
