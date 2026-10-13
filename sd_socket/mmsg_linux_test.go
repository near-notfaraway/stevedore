//go:build linux

package sd_socket

import (
	"testing"
	"unsafe"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestMMsgContainersLinux(t *testing.T) {
	recv := NewMMsgContainer(1, 1)
	require.Len(t, recv.mMsg, minMMsgContainerBatchNum)
	require.Len(t, recv.ioVecs, minMMsgContainerBatchNum)
	require.Len(t, recv.buffers, minMMsgContainerBatchNum)
	require.Len(t, recv.names, minMMsgContainerBatchNum)
	require.Len(t, recv.buffers[0], minMMsgContainerMTU)
	require.Equal(t, uint32(sizeofSockaddrInet6), recv.mMsg[0].hdr.nameLen)
	require.Equal(t, uint64(1), recv.mMsg[0].hdr.iovLen)

	recv.mMsg[0].len = 7
	require.Equal(t, uint32(7), recv.GetLengthOfMsg(0))
	require.Same(t, &recv.buffers[0][0], &recv.GetBufOfMsg(0)[0])

	recv.mMsg[0].hdr.nameLen = sizeofSockaddrInet4
	copy(recv.names[0][4:8], []byte{192, 0, 2, 1})
	key := recv.GetRSockaddrKeyOfMsg(0)
	require.Equal(t, uint32(sizeofSockaddrInet4), key.nameLen)
	require.Equal(t, []byte{192, 0, 2, 1}, key.name[4:8])

	recv.ResetNameLengths()
	for i := range recv.mMsg {
		require.Equal(t, uint32(sizeofSockaddrInet6), recv.mMsg[i].hdr.nameLen)
	}

	send := NewSendMMsgContainer(1)
	require.Len(t, send.mMsg, minMMsgContainerBatchNum)
	require.Len(t, send.ioVecs, minMMsgContainerBatchNum)
	require.Len(t, send.names, minMMsgContainerBatchNum)

	payload := []byte("payload")
	send.Prepare(0, payload, key)
	require.Equal(t, uint64(len(payload)), send.ioVecs[0].len)
	require.NotNil(t, send.ioVecs[0].base)
	require.Equal(t, key.nameLen, send.mMsg[0].hdr.nameLen)
	require.Equal(t, key.name[:key.nameLen], send.names[0][:key.nameLen])

	send.Prepare(1, nil, SockaddrKey{})
	require.Zero(t, send.ioVecs[1].len)
	require.Nil(t, send.ioVecs[1].base)
}

func TestRecvMMsgLinuxReceivesBatch(t *testing.T) {
	receiver, receiverAddr := newLinuxUDPSocket(t, true)
	sender, _ := newLinuxUDPSocket(t, false)

	payloads := [][]byte{[]byte("first"), []byte("second"), []byte("third")}
	for _, payload := range payloads {
		require.NoError(t, unix.Sendto(sender, payload, 0, receiverAddr))
	}

	container := NewMMsgContainer(len(payloads), 128)
	count, errno := RecvMMsg(receiver, container)
	require.Zero(t, errno)
	require.Equal(t, len(payloads), count)

	for i, payload := range payloads {
		length := container.GetLengthOfMsg(i)
		require.Equal(t, payload, container.GetBufOfMsg(i)[:length])
		require.Equal(t, uint32(sizeofSockaddrInet4), container.GetRSockaddrKeyOfMsg(i).nameLen)
	}

	count, errno = RecvMMsg(receiver, container)
	require.Equal(t, -1, count)
	require.Equal(t, unix.EAGAIN, errno)

	count, errno = RecvMMsg(-1, container)
	require.Equal(t, -1, count)
	require.Equal(t, unix.EBADF, errno)
}

func TestSendMMsgLinuxSendsPreparedRange(t *testing.T) {
	receiver, receiverAddr := newLinuxUDPSocket(t, true)
	sender, _ := newLinuxUDPSocket(t, false)
	key := linuxSockaddrKey(t, receiverAddr)

	container := NewSendMMsgContainer(3)
	container.Prepare(0, []byte("skip"), key)
	container.Prepare(1, []byte("first"), key)
	container.Prepare(2, []byte("second"), key)

	count, err := SendMMsg(sender, container, 1, 3)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	assertLinuxDatagram(t, receiver, "first")
	assertLinuxDatagram(t, receiver, "second")

	require.NoError(t, SendMsg(sender, container, 0))
	assertLinuxDatagram(t, receiver, "skip")
}

func TestMMsgLinuxValidatesArguments(t *testing.T) {
	container := NewSendMMsgContainer(2)
	for _, bounds := range [][2]int{{-1, 0}, {1, 0}, {0, len(container.mMsg) + 1}} {
		sent, err := SendMMsg(-1, container, bounds[0], bounds[1])
		require.Zero(t, sent)
		require.ErrorIs(t, err, unix.EINVAL)
	}

	sent, err := SendMMsg(-1, container, 0, 0)
	require.NoError(t, err)
	require.Zero(t, sent)
	require.ErrorIs(t, SendMsg(-1, container, -1), unix.EINVAL)
	require.ErrorIs(t, SendMsg(-1, container, len(container.mMsg)), unix.EINVAL)

	key := linuxSockaddrKey(t, &unix.SockaddrInet4{
		Addr: [4]byte{127, 0, 0, 1},
		Port: 9,
	})
	container.Prepare(0, []byte("closed socket"), key)
	require.ErrorIs(t, SendMsg(-1, container, 0), unix.EBADF)
}

func linuxSockaddrKey(t *testing.T, addr unix.Sockaddr) SockaddrKey {
	t.Helper()

	name, nameLen := sockaddrToNameBuffer(addr)
	require.NotNil(t, name)
	*(*uint16)(unsafe.Pointer(&name[0])) = uint16(unix.AF_INET)

	var key SockaddrKey
	key.nameLen = nameLen
	copy(key.name[:], name)
	return key
}

func newLinuxUDPSocket(t *testing.T, nonblock bool) (int, unix.Sockaddr) {
	t.Helper()

	socketType := unix.SOCK_DGRAM | unix.SOCK_CLOEXEC
	if nonblock {
		socketType |= unix.SOCK_NONBLOCK
	}
	fd, err := unix.Socket(unix.AF_INET, socketType, unix.IPPROTO_UDP)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd) })
	require.NoError(t, unix.Bind(fd, &unix.SockaddrInet4{
		Addr: [4]byte{127, 0, 0, 1},
	}))
	addr, err := unix.Getsockname(fd)
	require.NoError(t, err)
	return fd, addr
}

func assertLinuxDatagram(t *testing.T, fd int, expected string) {
	t.Helper()

	var buf [128]byte
	n, _, err := unix.Recvfrom(fd, buf[:], 0)
	require.NoError(t, err)
	require.Equal(t, expected, string(buf[:n]))
}

func TestNewMMsgContainerConvey(t *testing.T) {
	convey.Convey("Test NewMMsgContainer", t, func() {
		convey.Convey("initializes kernel message headers", func() {
			container := NewMMsgContainer(1, 1)
			convey.So(container.mMsg, convey.ShouldHaveLength, minMMsgContainerBatchNum)
			convey.So(container.mMsg[0].hdr.iovLen, convey.ShouldEqual, 1)
		})
	})
}

func TestMMsgContainerGetLengthOfMsgConvey(t *testing.T) {
	convey.Convey("Test MMsgContainer.GetLengthOfMsg", t, func() {
		convey.Convey("returns the kernel length", func() {
			container := NewMMsgContainer(1, 1)
			container.mMsg[0].len = 7
			convey.So(container.GetLengthOfMsg(0), convey.ShouldEqual, 7)
		})
	})
}

func TestMMsgContainerGetBufOfMsgConvey(t *testing.T) {
	convey.Convey("Test MMsgContainer.GetBufOfMsg", t, func() {
		convey.Convey("returns the receive buffer", func() {
			container := NewMMsgContainer(1, 1)
			convey.So(container.GetBufOfMsg(0), convey.ShouldResemble, container.buffers[0])
		})
	})
}

func TestMMsgContainerResetNameLengthsConvey(t *testing.T) {
	convey.Convey("Test MMsgContainer.ResetNameLengths", t, func() {
		convey.Convey("restores address capacity", func() {
			container := NewMMsgContainer(1, 1)
			container.mMsg[0].hdr.nameLen = 0
			container.ResetNameLengths()
			convey.So(container.mMsg[0].hdr.nameLen, convey.ShouldEqual, sizeofSockaddrInet6)
		})
	})
}

func TestMMsgContainerGetRSockaddrKeyOfMsgConvey(t *testing.T) {
	convey.Convey("Test MMsgContainer.GetRSockaddrKeyOfMsg", t, func() {
		convey.Convey("copies the address key", func() {
			container := NewMMsgContainer(1, 1)
			container.mMsg[0].hdr.nameLen = sizeofSockaddrInet4
			copy(container.names[0][4:8], []byte{127, 0, 0, 1})
			key := container.GetRSockaddrKeyOfMsg(0)
			convey.So(key.nameLen, convey.ShouldEqual, sizeofSockaddrInet4)
			convey.So(key.name[4:8], convey.ShouldResemble, []byte{127, 0, 0, 1})
		})
	})
}

func TestNewSendMMsgContainerConvey(t *testing.T) {
	convey.Convey("Test NewSendMMsgContainer", t, func() {
		convey.Convey("initializes send headers", func() {
			container := NewSendMMsgContainer(1)
			convey.So(container.mMsg, convey.ShouldHaveLength, minMMsgContainerBatchNum)
			convey.So(container.mMsg[0].hdr.iovLen, convey.ShouldEqual, 1)
		})
	})
}

func TestSendMMsgContainerPrepareConvey(t *testing.T) {
	convey.Convey("Test SendMMsgContainer.Prepare", t, func() {
		convey.Convey("binds payload and address", func() {
			container := NewSendMMsgContainer(1)
			key := SockaddrKey{nameLen: sizeofSockaddrInet4}
			container.Prepare(0, []byte("payload"), key)
			convey.So(container.ioVecs[0].len, convey.ShouldEqual, len("payload"))
			convey.So(container.mMsg[0].hdr.nameLen, convey.ShouldEqual, sizeofSockaddrInet4)
		})
	})
}

func TestRecvMMsgConvey(t *testing.T) {
	convey.Convey("Test RecvMMsg", t, func() {
		convey.Convey("returns an invalid descriptor error", func() {
			count, errno := RecvMMsg(-1, NewMMsgContainer(1, 1))
			convey.So(count, convey.ShouldEqual, -1)
			convey.So(errno, convey.ShouldEqual, unix.EBADF)
		})
	})
}

func TestSendMMsgConvey(t *testing.T) {
	convey.Convey("Test SendMMsg", t, func() {
		convey.Convey("validates its range", func() {
			count, err := SendMMsg(-1, NewSendMMsgContainer(1), -1, 0)
			convey.So(count, convey.ShouldEqual, 0)
			convey.So(err, convey.ShouldEqual, unix.EINVAL)
		})
	})
}

func TestSendMsgConvey(t *testing.T) {
	convey.Convey("Test SendMsg", t, func() {
		convey.Convey("validates its index", func() {
			convey.So(SendMsg(-1, NewSendMMsgContainer(1), -1), convey.ShouldEqual, unix.EINVAL)
		})
	})
}
