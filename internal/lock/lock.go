// Package lock provides a runtime lock to prevent concurrent appimg invocations.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/adrg/xdg"
)

const lockFile = "appimg.lock"

// Lock holds a process-exclusive flock on the appimg runtime lock file.
type Lock struct {
	file *os.File
}

// Acquire exclusively flocks the runtime lock file, returning an error if
// another appimg instance is already running.
func Acquire() (*Lock, error) {
	path := filepath.Join(xdg.RuntimeDir, lockFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("another instance of appimg is already running")
	}
	return &Lock{file: f}, nil
}

// Release unlocks and removes the lock file.
func (l *Lock) Release() error {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	name := l.file.Name()
	if err := l.file.Close(); err != nil {
		return err
	}
	return os.Remove(name)
}
