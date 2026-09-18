//go:build darwin && cgo

package clipboard

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type delayedDAVReader struct {
	*bytes.Reader
	next <-chan struct{}
}

func (r *delayedDAVReader) Read(p []byte) (int, error) {
	if r.Len() < 4096 {
		<-r.next
	}
	if len(p) > 1024 {
		p = p[:1024]
	}
	return r.Reader.Read(p)
}

func TestDAVDeliversBeforeNextReadCompletes(t *testing.T) {
	next := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(&davProgressWriter{ResponseWriter: w}, r, "test.bin", time.Time{}, &delayedDAVReader{bytes.NewReader(bytes.Repeat([]byte{7}, 4096)), next})
	}))
	defer server.Close()
	// LIFO：先解除延遲，才能關閉 server。
	defer func() {
		select {
		case <-next:
		default:
			close(next)
		}
	}()
	done := make(chan error, 1)
	go func() {
		client := http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(server.URL)
		if err != nil {
			done <- err
			return
		}
		defer resp.Body.Close()
		data := make([]byte, 1024)
		_, err = io.ReadFull(resp.Body, data)
		if err == nil && !bytes.Equal(data, bytes.Repeat([]byte{7}, 1024)) {
			err = io.ErrUnexpectedEOF
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("第一段資料被後續讀取阻塞")
	}
}
func TestDAVRangesAndHeadRemainCorrect(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		req := httptest.NewRequest(method, "http://localhost/file", nil)
		req.Header.Set("Range", "bytes=3-7")
		out := httptest.NewRecorder()
		out.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(&davProgressWriter{ResponseWriter: out}, req, "test.bin", time.Time{}, bytes.NewReader([]byte("0123456789")))
		if out.Code != 206 || out.Header().Get("Content-Range") != "bytes 3-7/10" {
			t.Fatalf("%s: %d %v", method, out.Code, out.Header())
		}
		expected := "34567"
		if method == "HEAD" {
			expected = ""
		}
		if out.Body.String() != expected {
			t.Fatalf("%s: %q", method, out.Body.String())
		}
	}
}
