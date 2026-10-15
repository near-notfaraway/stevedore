package sd_util

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"

	"golang.org/x/sys/unix"
)

// PidFile holds the exclusive lock proving ownership of one running instance.
type PidFile struct {
	path      string
	file      *os.File
	closeOnce sync.Once
	closeErr  error
}

// AcquirePidFile locks the PID file and records ownership for the process lifetime.
func AcquirePidFile(path string) (*PidFile, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open pid file failed: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock pid file failed: %w", err)
	}
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("truncate pid file failed: %w", err)
	}
	if _, err := fmt.Fprintf(file, "%d", os.Getpid()); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write pid file failed: %w", err)
	}
	return &PidFile{path: path, file: file}, nil
}

// Close releases ownership and removes the PID file if it still belongs to this process.
func (p *PidFile) Close() error {
	p.closeOnce.Do(func() {
		if pid, err := GetPid(p.path); err != nil {
			p.closeErr = err
		} else if pid != os.Getpid() {
			p.closeErr = fmt.Errorf("pid file owner changed to %d", pid)
		} else if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) {
			p.closeErr = err
		}
		if err := p.file.Close(); err != nil && p.closeErr == nil {
			p.closeErr = err
		}
	})
	return p.closeErr
}

// GetRunningPid returns the recorded PID only when that process is still alive.
func GetRunningPid(pidFile string) (int, error) {
	file, err := os.OpenFile(pidFile, os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return -1, nil
		}
		return -1, fmt.Errorf("open pid file failed: %w", err)
	}
	defer file.Close()

	lockErr := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if lockErr == nil {
		return -1, nil
	}
	if !errors.Is(lockErr, unix.EWOULDBLOCK) && !errors.Is(lockErr, unix.EAGAIN) {
		return -1, fmt.Errorf("check pid file lock failed: %w", lockErr)
	}

	value, err := io.ReadAll(file)
	if err != nil {
		return -1, fmt.Errorf("read pid file failed: %w", err)
	}
	pid, err := parsePid(value)
	if err != nil {
		return -1, err
	}
	if pid <= 1 {
		return -1, fmt.Errorf("pid %d in file is invalid", pid)
	}
	return pid, nil
}

// GetPid reads a PID file and returns -1 when it does not exist.
func GetPid(pidFile string) (int, error) {
	value, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return -1, nil
		}
		return -1, fmt.Errorf("read pid file failed: %w", err)
	}

	return parsePid(value)
}

func parsePid(value []byte) (int, error) {
	pid, err := strconv.Atoi(string(value))
	if err != nil {
		return -1, fmt.Errorf("pid %s in file is invalid", string(value))
	}
	return pid, nil
}
