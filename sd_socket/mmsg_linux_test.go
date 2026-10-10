//go:build linux

package sd_socket

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestMMsgContainersLinux(t *testing.T) {
	recv := NewMMsgContainer(1, 1)
	require.Len(t, recv.MMsg, MinMMsgContainerBatchNum)
	require.Len(t, recv.IOVecs, MinMMsgContainerBatchNum)
	require.Len(t, recv.Buffers, MinMMsgContainerBatchNum)
	require.Len(t, recv.Names, MinMMsgContainerBatchNum)
	require.Len(t, recv.Buffers[0], MinMMsgContainerMTU)
	require.Equal(t, uint32(SizeofSockaddrInet6), recv.MMsg[0].Hdr.NameLen)
	require.Equal(t, uint64(1), recv.MMsg[0].Hdr.IOVLen)

	recv.MMsg[0].Len = 7
	require.Equal(t, uint32(7), recv.GetLengthOfMsg(0))
	require.Same(t, &recv.Buffers[0][0], &recv.GetBufOfMsg(0)[0])

	recv.MMsg[0].Hdr.NameLen = SizeofSockaddrInet4
	copy(recv.Names[0][4:8], []byte{192, 0, 2, 1})
	key := recv.GetRSockaddrKeyOfMsg(0)
	require.Equal(t, uint32(SizeofSockaddrInet4), key.NameLen)
	require.Equal(t, []byte{192, 0, 2, 1}, key.Name[4:8])

	recv.ResetNameLengths()
	for i := range recv.MMsg {
		require.Equal(t, uint32(SizeofSockaddrInet6), recv.MMsg[i].Hdr.NameLen)
	}

	send := NewSendMMsgContainer(1)
	require.Len(t, send.MMsg, MinMMsgContainerBatchNum)
	require.Len(t, send.IOVecs, MinMMsgContainerBatchNum)
	require.Len(t, send.Names, MinMMsgContainerBatchNum)

	payload := []byte("payload")
	send.Prepare(0, payload, key)
	require.Equal(t, uint64(len(payload)), send.IOVecs[0].Len)
	require.NotNil(t, send.IOVecs[0].Base)
	require.Equal(t, key.NameLen, send.MMsg[0].Hdr.NameLen)
	require.Equal(t, key.Name[:key.NameLen], send.Names[0][:key.NameLen])

	send.Prepare(1, nil, SockaddrKey{})
	require.Zero(t, send.IOVecs[1].Len)
	require.Nil(t, send.IOVecs[1].Base)
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
		require.Equal(t, uint32(SizeofSockaddrInet4), container.GetRSockaddrKeyOfMsg(i).NameLen)
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
	for _, bounds := range [][2]int{{-1, 0}, {1, 0}, {0, len(container.MMsg) + 1}} {
		sent, err := SendMMsg(-1, container, bounds[0], bounds[1])
		require.Zero(t, sent)
		require.ErrorIs(t, err, unix.EINVAL)
	}

	sent, err := SendMMsg(-1, container, 0, 0)
	require.NoError(t, err)
	require.Zero(t, sent)
	require.ErrorIs(t, SendMsg(-1, container, -1), unix.EINVAL)
	require.ErrorIs(t, SendMsg(-1, container, len(container.MMsg)), unix.EINVAL)

	key := linuxSockaddrKey(t, &unix.SockaddrInet4{
		Addr: [4]byte{127, 0, 0, 1},
		Port: 9,
	})
	container.Prepare(0, []byte("closed socket"), key)
	require.ErrorIs(t, SendMsg(-1, container, 0), unix.EBADF)
}

func linuxSockaddrKey(t *testing.T, addr unix.Sockaddr) SockaddrKey {
	t.Helper()

	name, nameLen := SockaddrToNameBuffer(addr)
	require.NotNil(t, name)
	*(*uint16)(unsafe.Pointer(&name[0])) = uint16(unix.AF_INET)

	var key SockaddrKey
	key.NameLen = nameLen
	copy(key.Name[:], name)
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
