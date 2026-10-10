//go:build darwin
// +build darwin

package sd_socket

import (
	"io"

	"golang.org/x/sys/unix"
)

type MMsgContainer struct {
	Buffers [][]byte
	Lengths []uint32
	Keys    []SockaddrKey
}

type SendMMsgContainer struct {
	Buffers [][]byte
	Keys    []SockaddrKey
}

func NewMMsgContainer(batchNum, mtu int) *MMsgContainer {
	batchNum, mtu = normalizeMMsgSize(batchNum, mtu)

	buffers := make([][]byte, batchNum)
	for i := range buffers {
		buffers[i] = make([]byte, mtu)
	}
	return &MMsgContainer{
		Buffers: buffers,
		Lengths: make([]uint32, batchNum),
		Keys:    make([]SockaddrKey, batchNum),
	}
}

func NewSendMMsgContainer(batchNum int) *SendMMsgContainer {
	batchNum, _ = normalizeMMsgSize(batchNum, MinMMsgContainerMTU)
	return &SendMMsgContainer{
		Buffers: make([][]byte, batchNum),
		Keys:    make([]SockaddrKey, batchNum),
	}
}

func (m *SendMMsgContainer) Prepare(index int, data []byte, key SockaddrKey) {
	m.Buffers[index] = data
	m.Keys[index] = key
}

func (m *MMsgContainer) GetLengthOfMsg(index int) uint32 {
	return m.Lengths[index]
}

func (m *MMsgContainer) GetBufOfMsg(index int) []byte {
	return m.Buffers[index]
}

func (m *MMsgContainer) ResetNameLengths() {}

func (m *MMsgContainer) GetRSockaddrKeyOfMsg(index int) SockaddrKey {
	return m.Keys[index]
}

func RecvMMsg(fd int, container *MMsgContainer) (int, unix.Errno) {
	received := 0
	for received < len(container.Buffers) {
		n, from, err := unix.Recvfrom(fd, container.Buffers[received], 0)
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

		name, nameLen := SockaddrToNameBuffer(from)
		if name == nil {
			return received, unix.EAFNOSUPPORT
		}
		var key SockaddrKey
		key.NameLen = nameLen
		copy(key.Name[:], name)
		container.Lengths[received] = uint32(n)
		container.Keys[received] = key
		received++
	}
	return received, 0
}

func SendMMsg(fd int, container *SendMMsgContainer, start, end int) (int, error) {
	if start < 0 || end < start || end > len(container.Buffers) {
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
	if index < 0 || index >= len(container.Buffers) {
		return unix.EINVAL
	}
	return sendPreparedMsg(fd, container, index)
}

func sendPreparedMsg(fd int, container *SendMMsgContainer, index int) error {
	key := container.Keys[index]
	to := NameBufferToSockaddr(key.Name[:], key.NameLen)
	if to == nil {
		return unix.EINVAL
	}

	for {
		n, err := unix.SendmsgN(fd, container.Buffers[index], nil, to, 0)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		if n != len(container.Buffers[index]) {
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
