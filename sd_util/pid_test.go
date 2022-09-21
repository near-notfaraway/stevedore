package sd_util

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

const testFile = "./test.pid"

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

// Should save pid from file succeed
func TestSavePid(t *testing.T) {
	// save pid in file
	err := SavePid(testFile)
	assert.Nil(t, err)

	// get pid and clean
	pid, err := GetPid(testFile)
	assert.Nil(t, err)
	assert.Greater(t, pid, 0)
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("clean test pid file failed\n")
	}
}

// Should remove pid from file succeed
func TestRemovePid(t *testing.T) {
	// create pif file
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("create test pid file failed\n")
	}
	if nw, err := f.WriteString("5555"); nw != 4 || err != nil {
		t.Fatalf("write test pid file failed\n")
	}
	f.Close()

	// remove and confirm
	err = RemovePid(testFile)
	assert.Nil(t, err)
	f, err = os.Open(testFile)
	assert.True(t, os.IsNotExist(err))
}
