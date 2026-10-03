package clipboard

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestPullChunksPreserveWireData(t *testing.T) {
	const requestID = "0123456789abcdef0123456789abcdef"
	id, _ := hex.DecodeString(requestID)
	for _, size := range []int{0, 1, dataChunk - 1, dataChunk, dataChunk + 17, pullReadSize} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i*31 + i/101)
			}
			original := bytes.Clone(data)
			var packets [][]byte
			err := sendPullChunks(context.Background(), requestID, data, func(_ context.Context, wire []byte) error {
				// 與 Pion 相同：送出回傳前複製，下一片可覆用來源。
				packets = append(packets, bytes.Clone(wire))
				return nil
			})
			if err != nil || !bytes.Equal(data, original) {
				t.Fatalf("來源資料遭改寫：%v", err)
			}
			if len(packets) != (size+dataChunk-1)/dataChunk {
				t.Fatal("分片數改變")
			}
			clear(data)
			var received []byte
			for i, wire := range packets {
				if len(wire) != 17+min(size-i*dataChunk, dataChunk) || wire[0] != 2 || !bytes.Equal(wire[1:17], id) {
					t.Fatalf("第 %d 片標頭或長度錯誤", i)
				}
				received = append(received, wire[17:]...)
			}
			if !bytes.Equal(received, original) {
				t.Fatal("後續分片或來源覆寫影響已送出的資料")
			}
		})
	}
}

func TestPullChunksStopOnSendFailure(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		want := errors.New("send failed")
		if cancelled {
			want = context.Canceled
		}
		calls := 0
		err := sendPullChunks(ctx, "0123456789abcdef0123456789abcdef", make([]byte, pullReadSize), func(ctx context.Context, _ []byte) error {
			calls++
			if calls == 2 {
				if cancelled {
					cancel()
					return ctx.Err()
				}
				return want
			}
			return nil
		})
		cancel()
		if !errors.Is(err, want) || calls != 2 {
			t.Fatalf("送出失敗後仍傳送：calls=%d err=%v", calls, err)
		}
	}
}

func TestReapPullReleasesExpiredReferences(t *testing.T) {
	p := newPullState()
	s := &Sync{pull: p}
	var closed [4]int
	for i := range closed {
		offer := &pullOffer{id: fmt.Sprint(i)}
		offer.last.Store(time.Now().Add(-11 * time.Minute).UnixNano())
		p.cleanup = append(p.cleanup, pullLease{offer: offer, close: func() { closed[i]++ }})
	}
	p.latestOffer = "1"
	p.cleanup[3].offer.last.Store(time.Now().UnixNano())
	backing := p.cleanup[:cap(p.cleanup)]
	s.reapPull()
	if len(p.cleanup) != 2 || p.cleanup[0].offer.id != "1" || p.cleanup[1].offer.id != "3" || closed != [4]int{1, 0, 1, 0} {
		t.Fatalf("回收改變保留順序或關閉次數：%v", closed)
	}
	for _, lease := range backing[len(p.cleanup):] {
		if lease.offer != nil || lease.close != nil {
			t.Fatal("切片尾端仍保留過期清單或關閉函式")
		}
	}
	p.reaped = time.Time{}
	p.latestOffer = ""
	for _, lease := range p.cleanup {
		lease.offer.last.Store(time.Now().Add(-11 * time.Minute).UnixNano())
	}
	s.reapPull()
	if len(p.cleanup) != 0 || closed != [4]int{1, 1, 1, 1} {
		t.Fatalf("清單未完全回收：%v", closed)
	}
	for _, lease := range backing {
		if lease.offer != nil || lease.close != nil {
			t.Fatal("全數回收後仍持有參照")
		}
	}
}

func BenchmarkPullChunkEncoding(b *testing.B) {
	for _, size := range []int{dataChunk, dataChunk + 17, pullReadSize} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			data := make([]byte, size)
			ctx := context.Background()
			var received int
			send := func(_ context.Context, wire []byte) error { received += len(wire) - 17; return nil }
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := sendPullChunks(ctx, "0123456789abcdef0123456789abcdef", data, send); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if received != size*b.N {
				b.Fatal("傳輸量錯誤")
			}
		})
	}
}
