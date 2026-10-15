package sd_util

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

const testFile = "./test.pid"

func TestAcquirePidFileLocksAndRemovesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stevedore.pid")

	owner, err := AcquirePidFile(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = owner.Close()
	})

	pid, err := GetPid(path)
	require.NoError(t, err)
	require.Equal(t, os.Getpid(), pid)

	_, err = AcquirePidFile(path)
	require.Error(t, err)

	require.NoError(t, owner.Close())
	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

func TestGetRunningPidIgnoresStalePid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stevedore.pid")
	owner, err := AcquirePidFile(path)
	require.NoError(t, err)

	pid, err := GetRunningPid(path)
	require.NoError(t, err)
	require.Equal(t, os.Getpid(), pid)

	require.NoError(t, owner.Close())
	require.NoError(t, os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644))
	pid, err = GetRunningPid(path)
	require.NoError(t, err)
	require.Equal(t, -1, pid)
}

func TestGetRunningPidTrustsLockedPidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stevedore.pid")
	const lockedPID = 999999999
	require.NoError(t, os.WriteFile(path, []byte(strconv.Itoa(lockedPID)), 0o644))
	owner, err := os.OpenFile(path, os.O_RDWR, 0)
	require.NoError(t, err)
	require.NoError(t, unix.Flock(int(owner.Fd()), unix.LOCK_EX|unix.LOCK_NB))
	defer owner.Close()

	pid, err := GetRunningPid(path)

	require.NoError(t, err)
	require.Equal(t, lockedPID, pid)
}

// Should get pid from file correctly
func TestGetPid(t *testing.T) {
	// not created
	pid, err := GetPid(testFile)
	assert.Nil(t, err)
	assert.Equal(t, -1, pid)

	// invalid pid
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("create test pid file failed\n")
	}
	if nw, err := f.WriteString("abcd"); nw != 4 || err != nil {
		t.Fatalf("write test pid file failed\n")
	}
	f.Close()
	pid, err = GetPid(testFile)
	assert.NotNil(t, err)
	assert.Equal(t, -1, pid)
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("clean test pid file failed\n")
	}

	// valid pid
	f, err = os.Create(testFile)
	if err != nil {
		t.Fatalf("create test pid file failed\n")
	}
	if nw, err := f.WriteString("5555"); nw != 4 || err != nil {
		t.Fatalf("write test pid file failed\n")
	}
	f.Close()
	pid, err = GetPid(testFile)
	assert.Nil(t, err)
	assert.Equal(t, 5555, pid)
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("clean test pid file failed\n")
	}
}
