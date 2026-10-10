//go:build linux
// +build linux

package sd_socket

import "golang.org/x/sys/unix"

func ReusePortShardingSupported() bool {
	return true
}

func newUDPSocket(family int, nonblock bool) (int, error) {
	socketType := unix.SOCK_DGRAM | unix.SOCK_CLOEXEC
	if nonblock {
		socketType |= unix.SOCK_NONBLOCK
	}
	return unix.Socket(family, socketType, unix.IPPROTO_UDP)
}
