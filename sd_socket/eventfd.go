package sd_socket

import (
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

func NewEventFD() (int, error) {
	fd, err := unix.Eventfd(0, unix.EFD_NONBLOCK|unix.EFD_CLOEXEC)
	if err != nil {
		return -1, os.NewSyscallError("eventfd", err)
	}
	return fd, nil
}

func NotifyEventFD(fd int) error {
	value := uint64(1)
	buf := (*[8]byte)(unsafe.Pointer(&value))[:]
	for {
		_, err := unix.Write(fd, buf)
		if err == unix.EINTR {
			continue
		}
		if err == unix.EAGAIN {
			return nil
		}
		if err != nil {
			return os.NewSyscallError("eventfd write", err)
		}
		return nil
	}
}

func DrainEventFD(fd int) error {
	var value uint64
	buf := (*[8]byte)(unsafe.Pointer(&value))[:]
	for {
		_, err := unix.Read(fd, buf)
		if err == unix.EINTR {
			continue
		}
		if err == unix.EAGAIN {
			return nil
		}
		if err != nil {
			return os.NewSyscallError("eventfd read", err)
		}
		return nil
	}
}
