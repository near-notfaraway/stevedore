//go:build darwin
// +build darwin

package sd_socket

import (
	"io"

	"golang.org/x/sys/unix"
)

type MMsgContainer struct {
	buffers [][]byte
	lengths []uint32
	keys    []SockaddrKey
}

func NewMMsgContainer(batchNum, mtu int) *MMsgContainer {
	batchNum, mtu = normalizeMMsgSize(batchNum, mtu)

	buffers := make([][]byte, batchNum)
	for i := range buffers {
		buffers[i] = make([]byte, mtu)
	}
	return &MMsgContainer{
		buffers: buffers,
		lengths: make([]uint32, batchNum),
		keys:    make([]SockaddrKey, batchNum),
	}
}

func (m *MMsgContainer) GetLengthOfMsg(index int) uint32 {
	return m.lengths[index]
}

func (m *MMsgContainer) GetBufOfMsg(index int) []byte {
	return m.buffers[index]
}

func (m *MMsgContainer) ResetNameLengths() {}

func (m *MMsgContainer) GetRSockaddrKeyOfMsg(index int) SockaddrKey {
	return m.keys[index]
}

type SendMMsgContainer struct {
	buffers [][]byte
	keys    []SockaddrKey
}

func NewSendMMsgContainer(batchNum int) *SendMMsgContainer {
	batchNum, _ = normalizeMMsgSize(batchNum, minMMsgContainerMTU)
	return &SendMMsgContainer{
		buffers: make([][]byte, batchNum),
		keys:    make([]SockaddrKey, batchNum),
	}
}

func (m *SendMMsgContainer) Prepare(index int, data []byte, key SockaddrKey) {
	m.buffers[index] = data
	m.keys[index] = key
}

func RecvMMsg(fd int, container *MMsgContainer) (int, unix.Errno) {
	received := 0
	for received < len(container.buffers) {
		n, from, err := unix.Recvfrom(fd, container.buffers[received], 0)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			errno := asErrno(err)
			if received > 0 && (errno == unix.EAGAIN || errno == unix.EWOULDBLOCK) {
				return received, 0
			}
			return received, errno
		}

		name, nameLen := sockaddrToNameBuffer(from)
		if name == nil {
			return received, unix.EAFNOSUPPORT
		}
		var key SockaddrKey
		key.nameLen = nameLen
		copy(key.name[:], name)
		container.lengths[received] = uint32(n)
		container.keys[received] = key
		received++
	}
	return received, 0
}

func SendMMsg(fd int, container *SendMMsgContainer, start, end int) (int, error) {
	if start < 0 || end < start || end > len(container.buffers) {
		return 0, unix.EINVAL
	}

	sent := 0
	for index := start; index < end; index++ {
		if err := sendPreparedMsg(fd, container, index); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

func SendMsg(fd int, container *SendMMsgContainer, index int) error {
	if index < 0 || index >= len(container.buffers) {
		return unix.EINVAL
	}
	return sendPreparedMsg(fd, container, index)
}

func sendPreparedMsg(fd int, container *SendMMsgContainer, index int) error {
	key := container.keys[index]
	to := nameBufferToSockaddr(key.name[:], key.nameLen)
	if to == nil {
		return unix.EINVAL
	}

	for {
		n, err := unix.SendmsgN(fd, container.buffers[index], nil, to, 0)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		if n != len(container.buffers[index]) {
			return io.ErrShortWrite
		}
		return nil
	}
}

func asErrno(err error) unix.Errno {
	if errno, ok := err.(unix.Errno); ok {
		return errno
	}
	return unix.EIO
}
