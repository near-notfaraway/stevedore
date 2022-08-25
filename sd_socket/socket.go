package sd_socket

import (
	"fmt"
	"golang.org/x/sys/unix"
	"syscall"
)

func UDPBoundSocket(sa unix.Sockaddr, nonblock, reuseAddr, reusePort bool) (int, error) {
	var family int
	switch sa.(type) {
	case *unix.SockaddrInet4:
		family = unix.AF_INET
	case *unix.SockaddrInet6:
		family = unix.AF_INET6
	default:
		return -1, fmt.Errorf("sockaddr family is not inet")
	}

	fd, err := UDPSocket(family, nonblock, reuseAddr, reusePort)
	if err != nil {
		return fd, err
	}

	if err = unix.Bind(fd, sa); err != nil {
		return -1, fmt.Errorf("bind sockaddr failed: %w", err)
	}

	return fd, nil
}

func UDPSocket(family int, nonblock, reuseAddr, reusePort bool) (fd int, err error) {
	if (family != unix.AF_INET) && (family != unix.AF_INET6) {
		return -1, fmt.Errorf("family is not inet")
	}

	typ := unix.SOCK_DGRAM | unix.SOCK_CLOEXEC
	if nonblock {
		typ |= unix.SOCK_NONBLOCK
	}

	fd, err = unix.Socket(family, typ, unix.IPPROTO_UDP)
	if err != nil {
		return -1, fmt.Errorf("create socket failed: %w", err)
	}

	if reuseAddr {
		err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_REUSEADDR, 1)
		if err != nil {
			return -1, fmt.Errorf("setsockopt reuseaddr failed: %w", err)
		}
	}

	if reusePort {
		err = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		if err != nil {
			return -1, fmt.Errorf("setsockopt reuseport failed: %w", err)
		}
	}

	return fd, nil
}
