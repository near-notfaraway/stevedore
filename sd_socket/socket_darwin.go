//go:build darwin
// +build darwin

package sd_socket

import "golang.org/x/sys/unix"

func ReusePortShardingSupported() bool {
	return false
}

func newUDPSocket(family int, nonblock bool) (int, error) {
	fd, err := unix.Socket(family, unix.SOCK_DGRAM, unix.IPPROTO_UDP)
	if err != nil {
		return -1, err
	}
	unix.CloseOnExec(fd)

	if !nonblock {
		return fd, nil
	}
	if err = unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}
	return fd, nil
}
