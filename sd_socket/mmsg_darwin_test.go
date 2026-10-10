//go:build darwin
// +build darwin

package sd_socket

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestMMsgContainersDarwin(t *testing.T) {
	recv := NewMMsgContainer(1, 1)
	require.Len(t, recv.Buffers, MinMMsgContainerBatchNum)
	require.Len(t, recv.Lengths, MinMMsgContainerBatchNum)
	require.Len(t, recv.Keys, MinMMsgContainerBatchNum)
	require.Len(t, recv.Buffers[0], MinMMsgContainerMTU)

	recv.Lengths[0] = 7
	recv.ResetNameLengths()
	require.Equal(t, uint32(7), recv.GetLengthOfMsg(0))
	require.Same(t, &recv.Buffers[0][0], &recv.GetBufOfMsg(0)[0])

	send := NewSendMMsgContainer(1)
	require.Len(t, send.Buffers, MinMMsgContainerBatchNum)
	require.Len(t, send.Keys, MinMMsgContainerBatchNum)

	key := SockaddrKey{NameLen: SizeofSockaddrInet4}
	payload := []byte("payload")
	send.Prepare(0, payload, key)
	require.Equal(t, payload, send.Buffers[0])
	require.Equal(t, key, send.Keys[0])
}

func TestRecvMMsgDarwinReceivesBatch(t *testing.T) {
	receiver, receiverAddr := newDarwinUDPSocket(t, true)

	sender, _ := newDarwinUDPSocket(t, false)

	payloads := [][]byte{
		[]byte("first"),
		[]byte("second"),
		[]byte("third"),
	}
	for _, payload := range payloads {
		require.NoError(t, unix.Sendto(sender, payload, 0, receiverAddr))
	}

	container := NewMMsgContainer(len(payloads), 128)
	count, errno := RecvMMsg(receiver, container)
	require.Zero(t, errno)
	require.Equal(t, len(payloads), count)

	senderAddr, err := unix.Getsockname(sender)
	require.NoError(t, err)
	expectedKey := sockaddrKeyFromNameBuffer(t, senderAddr)

	for i, payload := range payloads {
		length := container.GetLengthOfMsg(i)
		require.Equal(t, payload, container.GetBufOfMsg(i)[:length])
		require.Equal(t, expectedKey, container.GetRSockaddrKeyOfMsg(i))
	}

	count, errno = RecvMMsg(receiver, container)
	require.Zero(t, count)
	require.Equal(t, unix.EAGAIN, errno)
}

func TestRecvMMsgDarwinReturnsSocketError(t *testing.T) {
	container := NewMMsgContainer(1, 1)
	count, errno := RecvMMsg(-1, container)
	require.Zero(t, count)
	require.Equal(t, unix.EBADF, errno)
}

func TestSendMMsgDarwinSendsPreparedRange(t *testing.T) {
	receiver, receiverAddr := newDarwinUDPSocket(t, false)

	sender, _ := newDarwinUDPSocket(t, false)

	key := sockaddrKeyFromNameBuffer(t, receiverAddr)

	container := NewSendMMsgContainer(3)
	container.Prepare(0, []byte("skip"), key)
	container.Prepare(1, []byte("first"), key)
	container.Prepare(2, []byte("second"), key)

	count, err := SendMMsg(sender, container, 1, 3)
	require.NoError(t, err)
	require.Equal(t, 2, count)

	assertDarwinDatagram(t, receiver, "first")
	assertDarwinDatagram(t, receiver, "second")

	require.NoError(t, SendMsg(sender, container, 0))
	assertDarwinDatagram(t, receiver, "skip")
}

func TestSendMMsgDarwinValidatesRangeAndMessage(t *testing.T) {
	container := NewSendMMsgContainer(2)

	tests := []struct {
		start int
		end   int
	}{
		{start: -1, end: 0},
		{start: 1, end: 0},
		{start: 0, end: len(container.Buffers) + 1},
	}
	for _, tt := range tests {
		sent, err := SendMMsg(-1, container, tt.start, tt.end)
		require.Zero(t, sent)
		require.ErrorIs(t, err, unix.EINVAL)
	}

	sent, err := SendMMsg(-1, container, 0, 0)
	require.NoError(t, err)
	require.Zero(t, sent)

	require.ErrorIs(t, SendMsg(-1, container, -1), unix.EINVAL)
	require.ErrorIs(t, SendMsg(-1, container, len(container.Buffers)), unix.EINVAL)

	container.Prepare(0, []byte("invalid address"), SockaddrKey{})
	require.ErrorIs(t, SendMsg(-1, container, 0), unix.EINVAL)

	validKey := sockaddrKeyFromNameBuffer(t, &unix.SockaddrInet4{
		Addr: [4]byte{127, 0, 0, 1},
		Port: 9,
	})
	container.Prepare(0, []byte("closed socket"), validKey)
	require.ErrorIs(t, SendMsg(-1, container, 0), unix.EBADF)
}

func TestAsErrnoDarwin(t *testing.T) {
	require.Equal(t, unix.EINVAL, asErrno(unix.EINVAL))
	require.Equal(t, unix.EIO, asErrno(errors.New("not an errno")))
}

func sockaddrKeyFromNameBuffer(t *testing.T, addr unix.Sockaddr) SockaddrKey {
	t.Helper()

	name, nameLen := SockaddrToNameBuffer(addr)
	require.NotNil(t, name)

	var key SockaddrKey
	key.NameLen = nameLen
	copy(key.Name[:], name)
	return key
}

func newDarwinUDPSocket(t *testing.T, nonblock bool) (int, unix.Sockaddr) {
	t.Helper()

	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, unix.IPPROTO_UDP)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd) })

	require.NoError(t, unix.Bind(fd, &unix.SockaddrInet4{
		Addr: [4]byte{127, 0, 0, 1},
	}))
	require.NoError(t, unix.SetNonblock(fd, nonblock))
	addr, err := unix.Getsockname(fd)
	require.NoError(t, err)
	return fd, addr
}

func assertDarwinDatagram(t *testing.T, fd int, expected string) {
	t.Helper()

	buf := make([]byte, 128)
	n, _, err := unix.Recvfrom(fd, buf, 0)
	require.NoError(t, err)
	require.Equal(t, expected, string(buf[:n]))
}
