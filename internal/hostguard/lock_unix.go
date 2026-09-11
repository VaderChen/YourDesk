//go:build darwin || linux

package hostguard

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func tryLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrOccupied
		}
		return nil, err
	}
	return func() { f.Close() }, nil
}
