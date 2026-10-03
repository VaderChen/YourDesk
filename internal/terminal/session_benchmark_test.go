package terminal

import (
	"fmt"
	"sync"
	"testing"
)

var outputBenchmarkResult any

func BenchmarkTerminalOutput(b *testing.B) {
	for _, backlog := range []int{0, 64 * 1024} {
		b.Run(fmt.Sprintf("Backlog%d", backlog), func(b *testing.B) {
			s := &session{}
			s.space = sync.NewCond(&s.mu)
			s.appendOutput(make([]byte, backlog))
			data := make([]byte, 4096)
			b.ReportAllocs()
			for b.Loop() {
				s.mu.Lock()
				s.appendOutput(data)
				result, err := s.readOutput(s.sequence)
				s.mu.Unlock()
				if err != nil {
					b.Fatal(err)
				}
				outputBenchmarkResult = result
			}
		})
	}
}
