package sd_socket

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

//------------------------------------------------------------------------------
// MMsg related struct which mapping by unix api
//------------------------------------------------------------------------------

// Message data buffer for read/write buffer
type IOVec struct {
	Base *byte
	Len  uint64
}

// The message which need read/write though socket fd
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

// The multi message warp
type MMsgHdr struct {
	Hdr     MsgHdr
	Len     uint32
	PadCgo0 [4]byte
}

// Control info in message header
type CMsgHdr struct {
	Len   uint64
	Level int32
	Type  int32
}

//------------------------------------------------------------------------------
// MMsg related struct which customized for easy to use
//------------------------------------------------------------------------------

// Use for mmsg syscall
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

type SockaddrKey struct {
	NameLen uint32
	Name    [SizeofSockaddrInet6]byte
}

func NewMMsgContainer(batchNum, mtu int) *MMsgContainer {
	if batchNum < MinMMsgContainerBatchNum {
		batchNum = MinMMsgContainerBatchNum
	}
	if mtu < MinMMsgContainerMTU {
		mtu = MinMMsgContainerMTU
	}

	mmsg := make([]MMsgHdr, batchNum)
	iovecs := make([]IOVec, batchNum)
	buffers := make([][]byte, batchNum)
	names := make([][]byte, batchNum)

	for i := range mmsg {
		buffers[i] = make([]byte, mtu)
		names[i] = make([]byte, SizeofSockaddrInet6)
		iovecs[i] = IOVec{
			Base: (*byte)(unsafe.Pointer(&buffers[i][0])),
			Len:  uint64(len(buffers[i])),
		}

		mmsg[i].Hdr.IOV = &iovecs[i]
		mmsg[i].Hdr.IOVLen = 1

		mmsg[i].Hdr.Name = (*byte)(unsafe.Pointer(&names[i][0]))
		mmsg[i].Hdr.NameLen = uint32(len(names[i]))

		// ignore mms[i].Hdr.Control and mms[i].Hdr.ControlLen
	}

	return &MMsgContainer{
		MMsg:    mmsg,
		IOVecs:  iovecs,
		Buffers: buffers,
		Names:   names,
	}
}

func NewSendMMsgContainer(batchNum int) *SendMMsgContainer {
	if batchNum < MinMMsgContainerBatchNum {
		batchNum = MinMMsgContainerBatchNum
	}
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

func (m *MMsgContainer) GetLengthOfMsg(seq int) uint32 {
	return m.MMsg[seq].Len
}

func (m *MMsgContainer) GetBufOfMsg(seq int) []byte {
	return m.Buffers[seq]
}

// ResetNameLengths restores msg_namelen after the container was used by another socket family.
func (m *MMsgContainer) ResetNameLengths() {
	for i := range m.MMsg {
		m.MMsg[i].Hdr.NameLen = uint32(len(m.Names[i]))
	}
}

func (m *MMsgContainer) GetRSockaddrKeyOfMsg(seq int) SockaddrKey {
	var key SockaddrKey
	key.NameLen = m.MMsg[seq].Hdr.NameLen
	copy(key.Name[:], m.Names[seq][:key.NameLen])
	return key
}

const (
	MinMMsgContainerBatchNum = 32
	MinMMsgContainerMTU      = 4096
)

//------------------------------------------------------------------------------
// MMsg related func which wrapped syscall
//------------------------------------------------------------------------------

func RecvMMsg(fd int, mc *MMsgContainer) (int, unix.Errno) {
	n, _, errno := unix.Syscall6(unix.SYS_RECVMMSG, uintptr(fd),
		uintptr(unsafe.Pointer(&mc.MMsg[0])), uintptr(len(mc.MMsg)), unix.MSG_WAITFORONE, 0, 0)

	return int(n), errno
}

func SendMMsg(fd int, messages []MMsgHdr) (int, error) {
	if len(messages) == 0 {
		return 0, nil
	}
	for {
		n, _, errno := unix.Syscall6(unix.SYS_SENDMMSG, uintptr(fd),
			uintptr(unsafe.Pointer(&messages[0])), uintptr(len(messages)), 0, 0, 0)
		if errno == unix.EINTR {
			continue
		}
		if errno != 0 {
			return int(n), errno
		}
		return int(n), nil
	}
}

func SendMsg(fd int, message *MMsgHdr) error {
	for {
		_, _, errno := unix.Syscall(unix.SYS_SENDMSG, uintptr(fd),
			uintptr(unsafe.Pointer(&message.Hdr)), 0)
		if errno == unix.EINTR {
			continue
		}
		if errno != 0 {
			return errno
		}
		return nil
	}
}
