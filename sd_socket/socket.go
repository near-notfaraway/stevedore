package sd_socket

import (
	"fmt"

	"golang.org/x/sys/unix"
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
		_ = unix.Close(fd)
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

	fd, err := newUDPSocket(family, nonblock)
	if err != nil {
		return -1, fmt.Errorf("create socket failed: %w", err)
	}

	if reuseAddr {
		err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
		if err != nil {
			_ = unix.Close(fd)
			return -1, fmt.Errorf("setsockopt reuseaddr failed: %w", err)
		}
	}

	if reusePort {
		err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
		if err != nil {
			_ = unix.Close(fd)
			return -1, fmt.Errorf("setsockopt reuseport failed: %w", err)
		}
	}

	return fd, nil
}

// Send data to specified addr through specified fd
// Continue if EINTR, return data already send
func SendTo(fd int, p []byte, flags int, to unix.Sockaddr) error {
	for {
		err := unix.Sendto(fd, p, flags, to)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return err
		}

		return nil
	}
}
