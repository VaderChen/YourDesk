package clientui

import (
	"context"
	"errors"
	"sync"
	"time"
)

var errFileDownloadSuspended = errors.New("下載已暫停")

// The worker owns the partial file and its confirmed write offset. UI methods
// only change this small state and cancel an in-flight HTTP request; they never
// wait for network or disk I/O. A pause can finish the already-started local
// write, after which the next progress event reports its confirmed byte count.
type fileDownloadControl struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	wake   chan struct{}

	state            string
	epoch, requestID uint64
	requestCancel    context.CancelFunc
	progress         fileProgress
	activeSince      time.Time
	activeOffset     int64
	lastEmit         time.Time
	onProgress       func(fileProgress)
}

func newFileDownloadControl(parent context.Context, progress func(fileProgress)) *fileDownloadControl {
	ctx, cancel := context.WithCancel(parent)
	return &fileDownloadControl{ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), state: "running", activeSince: time.Now(), onProgress: progress}
}

func (c *fileDownloadControl) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *fileDownloadControl) snapshotLocked() fileProgress {
	p := c.progress
	p.State = c.state
	p.Speed, p.ETA = 0, 0
	if c.state == "running" {
		elapsed := time.Since(c.activeSince).Seconds()
		if elapsed > 0 && p.Received > c.activeOffset {
			p.Speed = float64(p.Received-c.activeOffset) / elapsed
			if p.Total > p.Received {
				p.ETA = float64(p.Total-p.Received) / p.Speed
			}
		}
	}
	return p
}

func (c *fileDownloadControl) snapshot() fileProgress {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked()
}

func (c *fileDownloadControl) notify(force bool) {
	c.mu.Lock()
	if c.onProgress == nil || (!force && time.Since(c.lastEmit) < 200*time.Millisecond && c.progress.Received != c.progress.Total) || c.state == "cancelled" || c.state == "failed" {
		c.mu.Unlock()
		return
	}
	c.lastEmit = time.Now()
	c.progress.Sequence++
	p, callback := c.snapshotLocked(), c.onProgress
	c.mu.Unlock()
	// Notifications can originate on the UI thread and the worker. Sequence lets
	// the UI dispatcher discard an older event if callbacks cross in flight.
	callback(p)
}

func (c *fileDownloadControl) update(name string, received, total int64) {
	c.mu.Lock()
	c.progress.Name, c.progress.Received, c.progress.Total = name, received, total
	c.mu.Unlock()
	c.notify(false)
}

func (c *fileDownloadControl) pause() (fileProgress, error) {
	c.mu.Lock()
	if c.ctx.Err() != nil || c.state == "complete" || c.state == "failed" {
		c.mu.Unlock()
		return fileProgress{}, errors.New("目前沒有可暫停的下載")
	}
	c.state = "paused"
	c.epoch++
	cancel := c.requestCancel
	p := c.snapshotLocked()
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	c.notify(true)
	c.signal()
	return p, nil
}

func (c *fileDownloadControl) resume() error {
	c.mu.Lock()
	if c.ctx.Err() != nil || c.state == "complete" || c.state == "failed" {
		c.mu.Unlock()
		return errors.New("目前沒有可繼續的下載")
	}
	if c.state != "running" {
		c.state = "running"
		c.epoch++
		c.activeSince, c.activeOffset = time.Now(), c.progress.Received
	}
	c.mu.Unlock()
	c.signal()
	c.notify(true)
	return nil
}

func (c *fileDownloadControl) stop() {
	c.cancel()
	c.mu.Lock()
	c.state = "cancelled"
	c.epoch++
	c.mu.Unlock()
	c.signal()
}

func (c *fileDownloadControl) interrupt(epoch *uint64) {
	c.mu.Lock()
	if c.ctx.Err() != nil || c.state == "complete" || c.state == "failed" || (epoch != nil && (c.epoch != *epoch || c.state != "running")) {
		c.mu.Unlock()
		return
	}
	if c.state != "paused" {
		c.state = "interrupted"
	}
	c.epoch++
	cancel := c.requestCancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	c.notify(true)
	c.signal()
}

func (c *fileDownloadControl) waitRunning() (uint64, error) {
	for {
		if err := c.ctx.Err(); err != nil {
			return 0, err
		}
		c.mu.Lock()
		running, epoch := c.state == "running", c.epoch
		c.mu.Unlock()
		if running {
			return epoch, nil
		}
		select {
		case <-c.ctx.Done():
			return 0, c.ctx.Err()
		case <-c.wake:
		}
	}
}

func (c *fileDownloadControl) request(epoch uint64) (context.Context, func() bool, error) {
	c.mu.Lock()
	if c.ctx.Err() != nil {
		c.mu.Unlock()
		return nil, nil, c.ctx.Err()
	}
	if c.state != "running" || c.epoch != epoch {
		c.mu.Unlock()
		return nil, nil, errFileDownloadSuspended
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.requestID++
	id := c.requestID
	c.requestCancel = cancel
	c.mu.Unlock()
	return ctx, func() bool {
		cancel()
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.requestID == id {
			c.requestCancel = nil
		}
		return c.state == "running" && c.epoch == epoch && c.ctx.Err() == nil
	}, nil
}

func (c *fileDownloadControl) finish(err error) {
	c.mu.Lock()
	if err == nil && c.ctx.Err() == nil {
		c.state = "complete"
	} else if c.ctx.Err() == nil {
		c.state = "failed"
	}
	c.mu.Unlock()
	if err == nil {
		c.notify(true)
	}
}
