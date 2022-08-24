package sd_socket

import (
	"golang.org/x/sys/unix"
	"sync"
	"unsafe"
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
	Buffers [][]byte
	Names   [][]byte
}

func NewMMsgContainer(batchNum, mtu int) *MMsgContainer {
	mmsg := make([]MMsgHdr, batchNum)
	buffers := make([][]byte, batchNum)
	names := make([][]byte, batchNum)

	for i := range mmsg {
		buffers[i] = make([]byte, mtu)
		names[i] = make([]byte, SizeofSockaddrInet6)

		iov := []IOVec{{
			Base: (*byte)(unsafe.Pointer(&buffers[i][0])),
			Len:  uint64(len(buffers[i]))}}

		mmsg[i].Hdr.IOV = &iov[0]
		mmsg[i].Hdr.IOVLen = uint64(len(iov))

		mmsg[i].Hdr.Name = (*byte)(unsafe.Pointer(&names[i][0]))
		mmsg[i].Hdr.NameLen = uint32(len(names[i]))

		// ignore mms[i].Hdr.Control and mms[i].Hdr.ControlLen
	}

	return &MMsgContainer{
		mmsg,
		buffers,
		names,
	}
}

func (m *MMsgContainer) GetLengthOfMsg(seq int) uint32 {
	return m.MMsg[seq].Len
}

func (m *MMsgContainer) GetBufOfMsg(seq int) []byte {
	return m.Buffers[seq]
}

func (m *MMsgContainer) GetRNamesOfMsg(seq int) string {
	return string(m.Names[seq][:m.MMsg[seq].Hdr.NameLen])
}

func (m *MMsgContainer) GetRSockaddrOfMsg(seq int) unix.Sockaddr {
	return NameBufferToSockaddr(m.Names[seq], m.MMsg[seq].Hdr.NameLen)
}

const (
	MinMMsgContainerBatchNum = 32
	MinMMsgContainerMTU      = 4096
)

// Use for allocate MMsgContainer
type MMsgContainerPool struct {
	batchNum int
	mtu      int
	mcPool   sync.Pool
}

func NewMMsgContainerPool(batchNum, mtu int) *MMsgContainerPool {
	if batchNum < MinMMsgContainerBatchNum {
		batchNum = MinMMsgContainerBatchNum
	}

	if mtu < MinMMsgContainerMTU {
		mtu = MinMMsgContainerMTU
	}

	return &MMsgContainerPool{
		batchNum: batchNum,
		mtu:      mtu,
		mcPool: sync.Pool{
			New: func() interface{} {
				return NewMMsgContainer(batchNum, mtu)
			},
		},
	}
}

func (m *MMsgContainerPool) GetMMsgContainerFromPool() *MMsgContainer {
	return m.mcPool.Get().(*MMsgContainer)
}

func (m *MMsgContainerPool) PutMMsgContainerToPool(mc *MMsgContainer) {
	m.mcPool.Put(mc)
}

//------------------------------------------------------------------------------
// MMsg related func which wrapped syscall
//------------------------------------------------------------------------------

func RecvMMsg(fd int, mc *MMsgContainer) (int, error) {
	n, _, err := unix.Syscall6(unix.SYS_RECVMMSG, uintptr(fd),
		uintptr(unsafe.Pointer(&mc.MMsg[0])), uintptr(len(mc.MMsg)), 0, 0, 0)
	return int(n), err
}

func SendMMsg(fd int, mc *MMsgContainer) (int, error) {
	n, _, err := unix.Syscall6(unix.SYS_SENDMMSG, uintptr(fd),
		uintptr(unsafe.Pointer(&mc.MMsg[0])), uintptr(len(mc.MMsg)), 0, 0, 0)
	return int(n), err
}
