package lock

import (
	"os"
	"testing"

	"github.com/adrg/xdg"
)

func overrideXDGRuntimeDir(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	xdg.Reload()
	t.Cleanup(xdg.Reload)
}

func TestAcquireRelease(t *testing.T) {
	overrideXDGRuntimeDir(t)

	l, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestAcquire_AlreadyRunning(t *testing.T) {
	overrideXDGRuntimeDir(t)

	l, err := Acquire()
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer func() { _ = l.Release() }()

	if _, err := Acquire(); err == nil {
		t.Fatal("expected error for second Acquire, got nil")
	}
}

func TestRelease_RemovesLockFile(t *testing.T) {
	overrideXDGRuntimeDir(t)

	l, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	name := l.file.Name()
	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// Lock file should be gone after Release.
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Errorf("lock file still exists after Release: %s", name)
	}
}
