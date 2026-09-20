package p2p

import (
	"fmt"
	"testing"
)

func BenchmarkFrameAssembler(b *testing.B) {
	for _, size := range []int{4096, 256 * 1024, 1024 * 1024} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			total := (size + chunkSize - 1) / chunkSize
			packets := make([][]byte, total)
			for i := range packets {
				packets[i] = make([]byte, frameHeaderSize+min(chunkSize, size-i*chunkSize))
				putHeader(packets[i], 1, 1920, 1080, uint32(i), uint32(total))
			}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				var a frameAssembler
				for i, packet := range packets {
					frame, ok := a.add(packet)
					if ok != (i == total-1) || (ok && len(frame.JPEG) != size) {
						b.Fatal("invalid assembled frame")
					}
				}
			}
		})
	}
}
