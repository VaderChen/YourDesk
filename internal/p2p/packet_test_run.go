package p2p

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"yourdesk/internal/authlog"
)

type probePayload struct {
	Data []byte `json:"data"`
	Hash string `json:"hash"`
}
type probeReply struct {
	Data         []byte `json:"data"`
	Hash         string `json:"hash"`
	Received     int    `json:"received"`
	ReceivedHash string `json:"receivedHash"`
}

func probeHash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func answerProbe(raw json.RawMessage) (any, error) {
	if !authlog.IsEnabled() {
		return nil, fmt.Errorf("請在遠端開啟網路 Debug")
	}
	var in probePayload
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	if len(in.Data) < 1 || len(in.Data) > 4096 || probeHash(in.Data) != in.Hash {
		return nil, fmt.Errorf("測試資料長度或校驗無效")
	}
	data := make([]byte, len(in.Data))
	if _, err := rand.Read(data); err != nil {
		return nil, err
	}
	return probeReply{data, probeHash(data), len(in.Data), in.Hash}, nil
}

type PacketTestReport struct {
	Layers    []LayerProgress `json:"layers,omitempty"`
	Attempts  int             `json:"attempts"`
	Verified  int             `json:"verified"`
	Sent      int             `json:"sent"`
	Received  int             `json:"received"`
	RTTMillis float64         `json:"rttMillis"`
	Seconds   float64         `json:"seconds"`
	Error     string          `json:"error,omitempty"`
}

// 每批最多兩秒送出、四個在途請求；沿用授權連線與可靠通道，不宣稱量測 UDP 丟包率。
func (p *Peer) RunPacketTest(ctx context.Context) PacketTestReport {
	report := PacketTestReport{}
	if !authlog.IsEnabled() {
		report.Error = "網路 Debug 已關閉"
		return report
	}
	if !p.SupportsCommand("network.probe") {
		report.Error = "遠端版本不支援雙向封包測試，請更新兩端程式"
		return report
	}
	start := time.Now()
	until := start.Add(2 * time.Second)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var total time.Duration
	tracker := newLayerTracker()
	sample := func() {
		mu.Lock()
		attempts, verified := report.Attempts, report.Verified
		mu.Unlock()
		tracker.update(p.packetLayers(uint64(attempts), uint64(verified)))
	}
	sample()
	samplingDone := make(chan struct{})
	samplingStopped := make(chan struct{})
	go func() {
		defer close(samplingStopped)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-samplingDone:
				return
			case <-ticker.C:
				sample()
			}
		}
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(until) && ctx.Err() == nil && authlog.IsEnabled() {
				if !time.Now().Before(until) || !authlog.IsEnabled() {
					return
				}
				mu.Lock()
				n := report.Attempts
				report.Attempts++
				mu.Unlock()
				size := []int{256, 1200, 4096}[n%3]
				data := make([]byte, size)
				if _, err := rand.Read(data); err != nil {
					mu.Lock()
					report.Error = err.Error()
					mu.Unlock()
					cancel()
					return
				}
				hash := probeHash(data)
				raw, _ := json.Marshal(probePayload{data, hash})
				t := time.Now()
				response, err := p.CallCommandParams(ctx, "network.probe", raw)
				var reply probeReply
				if err == nil {
					err = json.Unmarshal(response.Result, &reply)
				}
				if err == nil && (reply.Received != size || reply.ReceivedHash != hash || len(reply.Data) != size || probeHash(reply.Data) != reply.Hash) {
					err = fmt.Errorf("雙向資料完整性校驗失敗")
				}
				mu.Lock()
				if err != nil {
					if report.Error == "" {
						report.Error = err.Error()
					}
					mu.Unlock()
					cancel()
					return
				}
				report.Verified++
				report.Sent += size
				report.Received += size
				total += time.Since(t)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	close(samplingDone)
	<-samplingStopped
	sample()
	report.Layers = tracker.result()
	report.Seconds = time.Since(start).Seconds()
	if report.Verified > 0 {
		report.RTTMillis = float64(total.Microseconds()) / 1000 / float64(report.Verified)
	}
	authlog.Event("packet-test-result", map[string]any{"layer": "L1", "report": report})
	return report
}
