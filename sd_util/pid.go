package sd_util

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
)

// Get pid from pid file
// Return (-1, error) if get failed,
// Return (-1, nil) if pid file not exist
func GetPid(pidFile string) (int, error) {
	pf, err := os.Open(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return -1, nil
		}
		return -1, fmt.Errorf("open pid file failed: %w", err)
	}
	defer pf.Close()

	p, _ := ioutil.ReadAll(pf)
	pid, err := strconv.Atoi(string(p))
	if err != nil {
		return -1, fmt.Errorf("pid %s in file is invalid", string(p))
	}

	return pid, nil
}

// Save pid to pid file
// Return error if save failed
func SavePid(pidFile string) error {
	pf, err := os.Create(pidFile)
	if err != nil {
		return fmt.Errorf("create pid file failed: %w", err)
	}
	defer pf.Close()

	pid := os.Getpid()
	_, err = pf.Write([]byte(fmt.Sprintf("%d", pid)))
	if err != nil {
		return fmt.Errorf("write pid file failed: %w", err)
	}

	return nil
}

// Remove the pid file
// Return error if remove failed
func RemovePid(pidFile string) error {
	return os.Remove(pidFile)
}
