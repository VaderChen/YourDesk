package clipboard

import "context"

type pasteTransferJob struct {
	done chan struct{}
	err  error // close(done) 後才讀取。
}

// 同一份等待工作共用背景傳輸，不因失焦取消按鍵而中止資料或重複排入傳輸。
func (s *Sync) waitPasteTransfer(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	job := s.pasteTransfer
	if job == nil {
		job = &pasteTransferJob{done: make(chan struct{})}
		s.pasteTransfer = job
		parent := s.ctx
		go func() {
			transferCtx, cancel := context.WithTimeout(parent, TransferTimeout)
			job.err = s.transferForPaste(transferCtx)
			cancel()
			s.mu.Lock()
			if s.pasteTransfer == job {
				s.pasteTransfer = nil
			}
			close(job.done)
			s.mu.Unlock()
		}()
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-job.done:
		return job.err
	}
}
