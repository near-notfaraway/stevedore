package sd_socket

import (
	"fmt"
	"golang.org/x/sys/unix"
	"syscall"
)

// Create a udp socket which bound addr, also support general flag
// Returns err if create fails
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

// Create a udp socket, also support general flag
// Returns err if create fails
func UDPSocket(family int, nonblock, reuseAddr, reusePort bool) (int, error) {
	if (family != unix.AF_INET) && (family != unix.AF_INET6) {
		return -1, fmt.Errorf("family is not inet")
	}

	typ := unix.SOCK_DGRAM | unix.SOCK_CLOEXEC
	if nonblock {
		typ |= unix.SOCK_NONBLOCK
	}

	fd, err := unix.Socket(family, typ, unix.IPPROTO_UDP)
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

// Set send or recv timeout for socket
// Returns err if set fails
func SetSocketTimeout(fd, sTimeoutSec, rTimeoutSec int) error {
	if sTimeoutSec > 0 {
		if err := syscall.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO,
			&syscall.Timeval{Sec: int64(sTimeoutSec)}); err != nil {
			return fmt.Errorf("set fd %s send timeout fail: %w", fd, err)
		}
	}

	if rTimeoutSec > 0 {
		if err := syscall.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO,
			&syscall.Timeval{Sec: int64(rTimeoutSec)}); err != nil {
			return fmt.Errorf("set fd %s recv timeout fail: %w",  fd, err)
		}
	}

	return nil
}
