package clientui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
)

func TestDownloadTruncatedResponseKeepsConfirmedBytes(t *testing.T) {
	content := make([]byte, 10001)
	for i := range content {
		content[i] = byte(i + (i/4096)*43)
	}
	var interrupted atomic.Bool
	client := controlledDownloadFixture(t, content, func(w http.ResponseWriter, _ *http.Request, in controlledDownloadRequest) bool {
		if in.Action != "read" || in.Params.Offset != 4096 || !interrupted.CompareAndSwap(false, true) {
			return false
		}
		connection, writer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return true
		}
		defer connection.Close()
		fmt.Fprint(writer, "HTTP/1.1 200 OK\r\nContent-Length: 512\r\n\r\n{\"data\":\"")
		_ = writer.Flush()
		return true
	})
	control, done := startControlledDownload(t, client)
	if progress := waitControlledState(t, control, "interrupted"); progress.Received != 4096 {
		t.Fatalf("truncated response changed confirmed offset: %+v", progress)
	}
	if err := control.resume(); err != nil {
		t.Fatal(err)
	}
	checkControlledBytes(t, awaitControlledResult(t, done), content)
}

func TestDownloadMalformedReusedBlockCleansPartial(t *testing.T) {
	for _, malformed := range []string{"base64", "short", "oversize", "bytes", "offset", "eof"} {
		t.Run(malformed, func(t *testing.T) {
			content := bytes.Repeat([]byte{9}, 9000)
			client := controlledDownloadFixture(t, content, func(w http.ResponseWriter, _ *http.Request, in controlledDownloadRequest) bool {
				if in.Action != "read" || in.Params.Offset != 4096 {
					return false
				}
				data := base64.StdEncoding.EncodeToString(content[4096:8192])
				size, offset, eof := 4096, 8192, false
				switch malformed {
				case "base64":
					data = data[:4000] + "!"
				case "short":
					data = base64.StdEncoding.EncodeToString(content[:1024])
				case "oversize":
					data = base64.StdEncoding.EncodeToString(content[:6000])
				case "bytes":
					size--
				case "offset":
					offset++
				case "eof":
					eof = true
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "bytes": size, "nextOffset": offset, "eof": eof})
				return true
			})
			control, done := startControlledDownload(t, client)
			assertFailedDownloadCleaned(t, client, control, done, "遠端檔案區塊不完整或已變更")
			if received := control.snapshot().Received; received != 4096 {
				t.Fatalf("malformed block was written: offset=%d", received)
			}
		})
	}
}

type downloadMemoryTransport func(*http.Request) (*http.Response, error)

func (transport downloadMemoryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func BenchmarkFileDownloadBuffers(b *testing.B) {
	const size = 1 << 20
	metadata, _ := json.Marshal(fileDownloadMetadata{Name: "benchmark.bin", Size: size, Modified: "v1"})
	block := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{19}, 4096))
	chunks := make([][]byte, size/4096)
	for i := range chunks {
		chunks[i], _ = json.Marshal(map[string]any{"data": block, "bytes": 4096, "nextOffset": (i + 1) * 4096, "eof": i == len(chunks)-1})
	}
	client := &fileDownloadClient{endpoint: "http://127.0.0.1:12345/api/files", downloads: b.TempDir(), client: &http.Client{Transport: downloadMemoryTransport(func(request *http.Request) (*http.Response, error) {
		defer request.Body.Close()
		var in controlledDownloadRequest
		if err := json.NewDecoder(request.Body).Decode(&in); err != nil {
			return nil, err
		}
		data := metadata
		if in.Action == "read" {
			index := in.Params.Offset / 4096
			if index < 0 || index >= int64(len(chunks)) {
				return nil, errors.New("invalid benchmark offset")
			}
			data = chunks[index]
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})}}
	b.SetBytes(size)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := client.prepare(context.Background(), "fixture.bin", nil)
		if err != nil {
			b.Fatal(err)
		}
		if err := os.Remove(out.Path); err != nil {
			b.Fatal(err)
		}
	}
}
