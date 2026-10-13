//go:build linux
// +build linux

package sd_socket

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

type ioVec struct {
	base *byte
	len  uint64
}

type msgHdr struct {
	name       *byte
	nameLen    uint32
	padCgo0    [4]byte
	iov        *ioVec
	iovLen     uint64
	control    *byte
	controlLen uint64
	flags      int32
	padCgo1    [4]byte
}

type mMsgHdr struct {
	hdr     msgHdr
	len     uint32
	padCgo0 [4]byte
}

type cMsgHdr struct {
	len   uint64
	level int32
	typ   int32
}

type MMsgContainer struct {
	mMsg    []mMsgHdr
	ioVecs  []ioVec
	buffers [][]byte
	names   [][]byte
}

func NewMMsgContainer(batchNum, mtu int) *MMsgContainer {
	batchNum, mtu = normalizeMMsgSize(batchNum, mtu)

	mmsg := make([]mMsgHdr, batchNum)
	iovecs := make([]ioVec, batchNum)
	buffers := make([][]byte, batchNum)
	names := make([][]byte, batchNum)

	for i := range mmsg {
		buffers[i] = make([]byte, mtu)
		names[i] = make([]byte, sizeofSockaddrInet6)
		iovecs[i] = ioVec{
			base: &buffers[i][0],
			len:  uint64(len(buffers[i])),
		}

		mmsg[i].hdr.iov = &iovecs[i]
		mmsg[i].hdr.iovLen = 1
		mmsg[i].hdr.name = &names[i][0]
		mmsg[i].hdr.nameLen = uint32(len(names[i]))
	}

	return &MMsgContainer{
		mMsg:    mmsg,
		ioVecs:  iovecs,
		buffers: buffers,
		names:   names,
	}
}

func (m *MMsgContainer) GetLengthOfMsg(index int) uint32 {
	return m.mMsg[index].len
}

func (m *MMsgContainer) GetBufOfMsg(index int) []byte {
	return m.buffers[index]
}

func (m *MMsgContainer) ResetNameLengths() {
	for i := range m.mMsg {
		m.mMsg[i].hdr.nameLen = uint32(len(m.names[i]))
	}
}

func (m *MMsgContainer) GetRSockaddrKeyOfMsg(index int) SockaddrKey {
	var key SockaddrKey
	key.nameLen = m.mMsg[index].hdr.nameLen
	copy(key.name[:], m.names[index][:key.nameLen])
	return key
}

type SendMMsgContainer struct {
	mMsg   []mMsgHdr
	ioVecs []ioVec
	names  [][sizeofSockaddrInet6]byte
}

func NewSendMMsgContainer(batchNum int) *SendMMsgContainer {
	batchNum, _ = normalizeMMsgSize(batchNum, minMMsgContainerMTU)

	mmsg := make([]mMsgHdr, batchNum)
	iovecs := make([]ioVec, batchNum)
	names := make([][sizeofSockaddrInet6]byte, batchNum)
	for i := range mmsg {
		mmsg[i].hdr.iov = &iovecs[i]
		mmsg[i].hdr.iovLen = 1
		mmsg[i].hdr.name = &names[i][0]
	}
	return &SendMMsgContainer{
		mMsg:   mmsg,
		ioVecs: iovecs,
		names:  names,
	}
}

func (m *SendMMsgContainer) Prepare(index int, data []byte, key SockaddrKey) {
	copy(m.names[index][:], key.name[:key.nameLen])
	m.mMsg[index].hdr.nameLen = key.nameLen
	m.mMsg[index].hdr.flags = 0
	m.mMsg[index].len = 0
	m.ioVecs[index].base = nil
	if len(data) > 0 {
		m.ioVecs[index].base = &data[0]
	}
	m.ioVecs[index].len = uint64(len(data))
}

func RecvMMsg(fd int, container *MMsgContainer) (int, unix.Errno) {
	n, _, errno := unix.Syscall6(
		unix.SYS_RECVMMSG,
		uintptr(fd),
		uintptr(unsafe.Pointer(&container.mMsg[0])),
		uintptr(len(container.mMsg)),
		unix.MSG_WAITFORONE,
		0,
		0,
	)
	return int(n), errno
}

func SendMMsg(fd int, container *SendMMsgContainer, start, end int) (int, error) {
	if start < 0 || end < start || end > len(container.mMsg) {
		return 0, unix.EINVAL
	}
	if start == end {
		return 0, nil
	}

	messages := container.mMsg[start:end]
	for {
		n, _, errno := unix.Syscall6(
			unix.SYS_SENDMMSG,
			uintptr(fd),
			uintptr(unsafe.Pointer(&messages[0])),
			uintptr(len(messages)),
			0,
			0,
			0,
		)
		if errno == unix.EINTR {
			continue
		}
		if errno != 0 {
			return 0, errno
		}
		return int(n), nil
	}
}

func SendMsg(fd int, container *SendMMsgContainer, index int) error {
	if index < 0 || index >= len(container.mMsg) {
		return unix.EINVAL
	}
	for {
		_, _, errno := unix.Syscall(
			unix.SYS_SENDMSG,
			uintptr(fd),
			uintptr(unsafe.Pointer(&container.mMsg[index].hdr)),
			0,
		)
		if errno == unix.EINTR {
			continue
		}
		if errno != 0 {
			return errno
		}
		return nil
	}
}
