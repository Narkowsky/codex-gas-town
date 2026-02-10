//go:build windows

package lock

// flockAcquire is a no-op on Windows. Gas Town doesn't run on Windows
// in production, so the advisory lock is not critical here.
func flockAcquire(path string) (func(), error) {
	return func() {}, nil
}
