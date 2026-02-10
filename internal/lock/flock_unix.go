//go:build !windows

package lock

import (
	"fmt"
	"os"
	"syscall"
)

// flockAcquire opens a flock file and acquires an exclusive advisory lock.
// Returns a cleanup function that releases the lock and closes the file.
// The flock prevents concurrent Acquire() calls from racing on the same lock path.
func flockAcquire(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644) //nolint:gosec // G304,G306: lock files are internal operational data
	if err != nil {
		return nil, fmt.Errorf("opening flock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquiring flock: %w", err)
	}

	cleanup := func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
		f.Close()
	}
	return cleanup, nil
}
