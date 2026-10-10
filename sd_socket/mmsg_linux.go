//go:build linux
// +build linux

package sd_socket

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

type IOVec struct {
	Base *byte
	Len  uint64
}

type MsgHdr struct {
	Name       *byte
	NameLen    uint32
	PadCgo0    [4]byte
	IOV        *IOVec
	IOVLen     uint64
	Control    *byte
	ControlLen uint64
	Flags      int32
	PadCgo1    [4]byte
}

type MMsgHdr struct {
	Hdr     MsgHdr
	Len     uint32
	PadCgo0 [4]byte
}

type CMsgHdr struct {
	Len   uint64
	Level int32
	Type  int32
}

type MMsgContainer struct {
	MMsg    []MMsgHdr
	IOVecs  []IOVec
	Buffers [][]byte
	Names   [][]byte
}

type SendMMsgContainer struct {
	MMsg   []MMsgHdr
	IOVecs []IOVec
	Names  [][SizeofSockaddrInet6]byte
}

func NewMMsgContainer(batchNum, mtu int) *MMsgContainer {
	batchNum, mtu = normalizeMMsgSize(batchNum, mtu)

	mmsg := make([]MMsgHdr, batchNum)
	iovecs := make([]IOVec, batchNum)
	buffers := make([][]byte, batchNum)
	names := make([][]byte, batchNum)

	for i := range mmsg {
		buffers[i] = make([]byte, mtu)
		names[i] = make([]byte, SizeofSockaddrInet6)
		iovecs[i] = IOVec{
			Base: &buffers[i][0],
			Len:  uint64(len(buffers[i])),
		}

		mmsg[i].Hdr.IOV = &iovecs[i]
		mmsg[i].Hdr.IOVLen = 1
		mmsg[i].Hdr.Name = &names[i][0]
		mmsg[i].Hdr.NameLen = uint32(len(names[i]))
	}

	return &MMsgContainer{
		MMsg:    mmsg,
		IOVecs:  iovecs,
		Buffers: buffers,
		Names:   names,
	}
}

func NewSendMMsgContainer(batchNum int) *SendMMsgContainer {
	batchNum, _ = normalizeMMsgSize(batchNum, MinMMsgContainerMTU)

	mmsg := make([]MMsgHdr, batchNum)
	iovecs := make([]IOVec, batchNum)
	names := make([][SizeofSockaddrInet6]byte, batchNum)
	for i := range mmsg {
		mmsg[i].Hdr.IOV = &iovecs[i]
		mmsg[i].Hdr.IOVLen = 1
		mmsg[i].Hdr.Name = &names[i][0]
	}
	return &SendMMsgContainer{
		MMsg:   mmsg,
		IOVecs: iovecs,
		Names:  names,
	}
}

func (m *SendMMsgContainer) Prepare(index int, data []byte, key SockaddrKey) {
	copy(m.Names[index][:], key.Name[:key.NameLen])
	m.MMsg[index].Hdr.NameLen = key.NameLen
	m.MMsg[index].Hdr.Flags = 0
	m.MMsg[index].Len = 0
	m.IOVecs[index].Base = nil
	if len(data) > 0 {
		m.IOVecs[index].Base = &data[0]
	}
	m.IOVecs[index].Len = uint64(len(data))
}

func (m *MMsgContainer) GetLengthOfMsg(index int) uint32 {
	return m.MMsg[index].Len
}

func (m *MMsgContainer) GetBufOfMsg(index int) []byte {
	return m.Buffers[index]
}

func (m *MMsgContainer) ResetNameLengths() {
	for i := range m.MMsg {
		m.MMsg[i].Hdr.NameLen = uint32(len(m.Names[i]))
	}
}

func (m *MMsgContainer) GetRSockaddrKeyOfMsg(index int) SockaddrKey {
	var key SockaddrKey
	key.NameLen = m.MMsg[index].Hdr.NameLen
	copy(key.Name[:], m.Names[index][:key.NameLen])
	return key
}

func RecvMMsg(fd int, container *MMsgContainer) (int, unix.Errno) {
	n, _, errno := unix.Syscall6(
		unix.SYS_RECVMMSG,
		uintptr(fd),
		uintptr(unsafe.Pointer(&container.MMsg[0])),
		uintptr(len(container.MMsg)),
		unix.MSG_WAITFORONE,
		0,
		0,
	)
	return int(n), errno
}

func SendMMsg(fd int, container *SendMMsgContainer, start, end int) (int, error) {
	if start < 0 || end < start || end > len(container.MMsg) {
		return 0, unix.EINVAL
	}
	if start == end {
		return 0, nil
	}

	messages := container.MMsg[start:end]
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
	if index < 0 || index >= len(container.MMsg) {
		return unix.EINVAL
	}
	for {
		_, _, errno := unix.Syscall(
			unix.SYS_SENDMSG,
			uintptr(fd),
			uintptr(unsafe.Pointer(&container.MMsg[index].Hdr)),
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
