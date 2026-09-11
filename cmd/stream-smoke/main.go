// stream-smoke 比較相同原生擷取、縮圖、編碼與 loopback TCP 工作的串行／雙緩衝。
// 不連外，不保存桌面影像；TCP ACK 僅確認資料完整抵達，不包含 遠端顯示 顯示。
package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"image"
	"io"
	"net"
	"os"
	"sort"
	"time"
	"yourdesk/internal/desktop"
	"yourdesk/internal/streampipeline"
	"yourdesk/internal/video"
)

type raw struct {
	image   image.Image
	id      int
	start   time.Time
	capture time.Duration
}
type packet struct {
	raw    raw
	bytes  []byte
	encode time.Duration
}
type result struct {
	Mode      string  `json:"mode"`
	Codec     string  `json:"codec"`
	Frames    int     `json:"frames"`
	FPS       float64 `json:"fps"`
	CaptureMS float64 `json:"captureMs"`
	EncodeMS  float64 `json:"encodeMs"`
	SendMS    float64 `json:"sendMs"`
	LatencyMS float64 `json:"latencyMs"`
	P95MS     float64 `json:"p95Ms"`
	Mbps      float64 `json:"mbps"`
}

func main() {
	n := flag.Int("frames", 60, "每次影格數")
	rounds := flag.Int("rounds", 3, "交替測量輪數")
	codec := flag.String("codec", "hardware-h264", "編碼器")
	fps := flag.Int("fps", 0, "擷取上限，0 表示不限速")
	live := flag.Bool("live", false, "使用持續擷取")
	flag.Parse()
	if *n < 1 || *rounds < 1 || *fps < 0 {
		panic("參數不正確")
	}
	for round := 0; round < *rounds; round++ {
		modes := []bool{false, true}
		if round%2 == 1 {
			modes = []bool{true, false}
		}
		for _, parallel := range modes {
			r, err := run(*n, *codec, *fps, parallel, *live)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			json.NewEncoder(os.Stdout).Encode(r)
		}
	}
}

func run(n int, codec string, fps int, parallel bool, live bool) (result, error) {
	r := result{Codec: codec, Mode: "serial"}
	if parallel {
		r.Mode = "double-buffer"
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return r, err
	}
	defer listener.Close()
	receiver := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			receiver <- err
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(2 * time.Minute))
		for expected := 0; expected < n; expected++ {
			var header [12]byte
			if _, err = io.ReadFull(conn, header[:]); err != nil {
				receiver <- err
				return
			}
			id, size, crc := binary.BigEndian.Uint32(header[:4]), binary.BigEndian.Uint32(header[4:8]), binary.BigEndian.Uint32(header[8:])
			if int(id) != expected || size > 64<<20 {
				receiver <- fmt.Errorf("影格順序或長度錯誤")
				return
			}
			data := make([]byte, size)
			if _, err = io.ReadFull(conn, data); err != nil {
				receiver <- err
				return
			}
			if crc32.ChecksumIEEE(data) != crc {
				receiver <- fmt.Errorf("緩衝資料遭覆寫")
				return
			}
			if _, err = conn.Write([]byte{1}); err != nil {
				receiver <- err
				return
			}
		}
		receiver <- nil
	}()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		return r, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Minute))
	var scaler desktop.StreamScaler
	defer scaler.Close()
	var encode func(image.Image, int) ([]byte, error)
	if codec == "software-jpeg" {
		encode = desktop.JPEG
	} else {
		encoder, e := video.NewIntraEncoder(video.Codec(codec))
		if e != nil {
			return r, e
		}
		defer encoder.Close()
		if gop, ok := encoder.(video.GOPEncoder); ok {
			gop.SetKeyframeInterval(10)
		}
		if rate, ok := encoder.(video.RateEncoder); ok {
			rate.SetRate(12_000_000, max(20, fps))
		}
		encode = encoder.Encode
	}
	var capturer desktop.Capturer = desktop.ScreenshotCapturer{}
	if live {
		value := desktop.NewLiveCapturer()
		defer value.Close()
		capturer = value
	}
	// 先暖機兩幀，避免將模型／原生編碼器初始化當成並行效益。
	warmUntil := time.Now().Add(10 * time.Second)
	for i := 0; i < 2; i++ {
		if time.Now().After(warmUntil) {
			return r, fmt.Errorf("暖機期間缺少新畫面，請操作桌面後重試")
		}
		im, e := capturer.Capture(0)
		if e != nil {
			if errors.Is(e, desktop.ErrNoNewFrame) {
				i--
				continue
			}
			return r, e
		}
		if _, e = encode(scaler.Scale(im, 1920, 1080), 80); e != nil {
			return r, e
		}
	}
	var ticker *time.Ticker
	if fps > 0 {
		ticker = time.NewTicker(time.Second / time.Duration(fps))
		defer ticker.Stop()
	}
	index := 0
	var totalBytes uint64
	var latencies []float64
	stages := streampipeline.Stages[raw, packet]{
		Capture: func(ctx context.Context) (raw, error) {
			if err := ctx.Err(); err != nil {
				return raw{}, err
			}
			if index >= n {
				return raw{}, io.EOF
			}
			if ticker != nil {
				select {
				case <-ctx.Done():
					return raw{}, ctx.Err()
				case <-ticker.C:
				}
			}
			started := time.Now()
			im, e := capturer.Capture(0)
			if errors.Is(e, desktop.ErrNoNewFrame) {
				return raw{}, streampipeline.Skip
			}
			value := raw{image: im, id: index, start: started, capture: time.Since(started)}
			index++
			return value, e
		},
		Encode: func(ctx context.Context, value raw) (packet, error) {
			started := time.Now()
			data, e := encode(scaler.Scale(value.image, 1920, 1080), 80)
			value.image = nil // 不讓傳送槽持有原始大圖。
			return packet{raw: value, bytes: data, encode: time.Since(started)}, e
		},
		Send: func(ctx context.Context, value packet) error {
			started := time.Now()
			var header [12]byte
			binary.BigEndian.PutUint32(header[:4], uint32(value.raw.id))
			binary.BigEndian.PutUint32(header[4:8], uint32(len(value.bytes)))
			binary.BigEndian.PutUint32(header[8:], crc32.ChecksumIEEE(value.bytes))
			buffers := net.Buffers{header[:], value.bytes}
			if _, e := buffers.WriteTo(conn); e != nil {
				return e
			}
			var ack [1]byte
			if _, e := io.ReadFull(conn, ack[:]); e != nil {
				return e
			}
			r.Frames++
			r.CaptureMS += float64(value.raw.capture) / 1e6
			r.EncodeMS += float64(value.encode) / 1e6
			r.SendMS += float64(time.Since(started)) / 1e6
			latencies = append(latencies, float64(time.Since(value.raw.start))/1e6)
			totalBytes += uint64(len(value.bytes))
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	started := time.Now()
	if parallel {
		err = streampipeline.Run(ctx, stages)
	} else {
		for i := 0; i < n; i++ {
			var a raw
			var b packet
			a, err = stages.Capture(ctx)
			if errors.Is(err, streampipeline.Skip) {
				i--
				continue
			}
			if err != nil {
				break
			}
			b, err = stages.Encode(ctx, a)
			if err != nil {
				break
			}
			err = stages.Send(ctx, b)
			if err != nil {
				break
			}
		}
	}
	elapsed := time.Since(started).Seconds()
	if err != nil {
		return r, err
	}
	if err = <-receiver; err != nil {
		return r, err
	}
	r.FPS = float64(r.Frames) / elapsed
	r.Mbps = float64(totalBytes) * 8 / elapsed / 1e6
	for _, latency := range latencies {
		r.LatencyMS += latency
	}
	r.CaptureMS /= float64(n)
	r.EncodeMS /= float64(n)
	r.SendMS /= float64(n)
	r.LatencyMS /= float64(n)
	sort.Float64s(latencies)
	r.P95MS = latencies[(len(latencies)-1)*95/100]
	return r, nil
}
