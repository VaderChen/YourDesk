package terminal

import (
	"bytes"
	"encoding/json"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func testOutputSession() *session {
	s := &session{}
	s.space = sync.NewCond(&s.mu)
	return s
}

func TestOutputAcknowledgementAndResponseOwnership(t *testing.T) {
	s := testOutputSession()
	initial := bytes.Repeat([]byte{'a'}, 8192)
	s.appendOutput(initial)
	first, err := s.readOutput(0)
	if err != nil || first.Sequence != 1 || !bytes.Equal(first.Data, initial[:4096]) {
		t.Fatal("invalid first output", err)
	}
	retry, err := s.readOutput(0)
	if err != nil || retry.Sequence != first.Sequence || !bytes.Equal(retry.Data, first.Data) || s.bufferedOutput() != 4096 {
		t.Fatal("retry consumed unacknowledged output", err)
	}
	if _, err := s.readOutput(2); err == nil || s.bufferedOutput() != 4096 {
		t.Fatal("future acknowledgement changed output")
	}
	s.appendOutput(bytes.Repeat([]byte{'b'}, 8192))
	s.ended = true
	s.failure = "closed"
	for s.bufferedOutput() > 0 {
		out, err := s.readOutput(s.sequence)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := json.Marshal(out)
		want, _ := json.Marshal(map[string]any{"data": out.Data, "ended": s.bufferedOutput() == 0, "error": "closed", "sequence": out.Sequence})
		if !bytes.Equal(got, want) {
			t.Fatalf("wire response changed: %s / %s", got, want)
		}
	}
	final, err := s.readOutput(s.sequence)
	if err != nil || final.Data != nil || !final.Ended || !bytes.Equal(first.Data, initial[:4096]) {
		t.Fatal("drain or retained response ownership changed", err)
	}
}

func TestOutputInterleavedWritesRemainBoundedAndOrdered(t *testing.T) {
	s := testOutputSession()
	random := rand.New(rand.NewSource(20261003))
	var expected []byte
	for i := 0; i < 20000; i++ {
		n := random.Intn(4096) + 1
		if len(expected)+n <= 256*1024 {
			data := make([]byte, n)
			_, _ = random.Read(data)
			s.appendOutput(data)
			expected = append(expected, data...)
		}
		if i%3 != 0 || len(expected) > 250*1024 {
			out, err := s.readOutput(s.sequence)
			n := min(len(expected), 4096)
			if err != nil || !bytes.Equal(out.Data, expected[:n]) {
				t.Fatalf("output reordered at %d: %v", i, err)
			}
			expected = expected[n:]
		}
		if cap(s.buffer) > 256*1024 || s.bufferedOutput() != len(expected) {
			t.Fatalf("unbounded/stale output at %d", i)
		}
	}
}

type outputConsole struct {
	data   chan []byte
	read   chan struct{}
	closed chan struct{}
	once   sync.Once
}

func (c *outputConsole) Read(b []byte) (int, error) {
	select {
	case data := <-c.data:
		n := copy(b, data)
		c.read <- struct{}{}
		return n, nil
	case <-c.closed:
		return 0, io.EOF
	}
}
func (c *outputConsole) Write(b []byte) (int, error) { return len(b), nil }
func (c *outputConsole) Resize(int, int) error       { return nil }
func (c *outputConsole) Wait() error                 { return nil }
func (c *outputConsole) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func TestFullOutputBufferCloseUnblocksReader(t *testing.T) {
	c := &outputConsole{data: make(chan []byte, 1), read: make(chan struct{}, 1), closed: make(chan struct{})}
	s := testOutputSession()
	s.tty, s.done = c, make(chan struct{})
	s.appendOutput(make([]byte, 256*1024))
	c.data <- []byte("more")
	done := make(chan struct{})
	go func() { s.read(); close(done) }()
	select {
	case <-c.read:
	case <-time.After(3 * time.Second):
		t.Fatal("console did not deliver output")
	}
	s.close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("close left a producer waiting for buffer space")
	}
	if s.bufferedOutput() != 256*1024 {
		t.Fatal("stopped reader appended beyond the buffer budget")
	}
}
