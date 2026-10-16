//go:build darwin
// +build darwin

package sd_socket

import (
	"errors"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func sockaddrKeyFromNameBuffer(t *testing.T, addr unix.Sockaddr) SockaddrKey {
	t.Helper()

	name, nameLen := sockaddrToNameBuffer(addr)
	require.NotNil(t, name)

	var key SockaddrKey
	key.nameLen = nameLen
	copy(key.name[:], name)
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

func TestNewMMsgContainerConvey(t *testing.T) {
	convey.Convey("Test NewMMsgContainer", t, func() {
		convey.Convey("initializes Darwin containers", func() {
			recv := NewMMsgContainer(1, 1)
			require.Len(t, recv.buffers, minMMsgContainerBatchNum)
			require.Len(t, recv.lengths, minMMsgContainerBatchNum)
			require.Len(t, recv.keys, minMMsgContainerBatchNum)
			require.Len(t, recv.buffers[0], minMMsgContainerMTU)

			recv.lengths[0] = 7
			recv.ResetNameLengths()
			require.Equal(t, uint32(7), recv.GetLengthOfMsg(0))
			require.Same(t, &recv.buffers[0][0], &recv.GetBufOfMsg(0)[0])

			send := NewSendMMsgContainer(1)
			require.Len(t, send.buffers, minMMsgContainerBatchNum)
			require.Len(t, send.keys, minMMsgContainerBatchNum)

			key := SockaddrKey{nameLen: sizeofSockaddrInet4}
			payload := []byte("payload")
			send.Prepare(0, payload, key)
			require.Equal(t, payload, send.buffers[0])
			require.Equal(t, key, send.keys[0])
		})
		convey.Convey("allocates receive storage", func() {
			container := NewMMsgContainer(1, 1)
			convey.So(container.buffers, convey.ShouldHaveLength, minMMsgContainerBatchNum)
			convey.So(container.buffers[0], convey.ShouldHaveLength, minMMsgContainerMTU)
		})
	})
}

func TestMMsgContainerGetLengthOfMsgConvey(t *testing.T) {
	convey.Convey("Test MMsgContainer.GetLengthOfMsg", t, func() {
		convey.Convey("returns recorded length", func() {
			container := NewMMsgContainer(1, 1)
			container.lengths[0] = 7
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
		convey.Convey("is a no-op on Darwin", func() {
			container := NewMMsgContainer(1, 1)
			container.lengths[0] = 3
			container.ResetNameLengths()
			convey.So(container.lengths[0], convey.ShouldEqual, 3)
		})
	})
}

func TestMMsgContainerGetRSockaddrKeyOfMsgConvey(t *testing.T) {
	convey.Convey("Test MMsgContainer.GetRSockaddrKeyOfMsg", t, func() {
		convey.Convey("returns the key", func() {
			container := NewMMsgContainer(1, 1)
			want := SockaddrKey{nameLen: sizeofSockaddrInet4}
			container.keys[0] = want
			convey.So(container.GetRSockaddrKeyOfMsg(0), convey.ShouldResemble, want)
		})
	})
}

func TestNewSendMMsgContainerConvey(t *testing.T) {
	convey.Convey("Test NewSendMMsgContainer", t, func() {
		convey.Convey("allocates send storage", func() {
			container := NewSendMMsgContainer(1)
			convey.So(container.buffers, convey.ShouldHaveLength, minMMsgContainerBatchNum)
			convey.So(container.keys, convey.ShouldHaveLength, minMMsgContainerBatchNum)
		})
	})
}

func TestSendMMsgContainerPrepareConvey(t *testing.T) {
	convey.Convey("Test SendMMsgContainer.Prepare", t, func() {
		convey.Convey("stores payload and key", func() {
			container := NewSendMMsgContainer(1)
			key := SockaddrKey{nameLen: sizeofSockaddrInet4}
			container.Prepare(0, []byte("payload"), key)
			convey.So(container.buffers[0], convey.ShouldResemble, []byte("payload"))
			convey.So(container.keys[0], convey.ShouldResemble, key)
		})
	})
}

func TestRecvMMsgConvey(t *testing.T) {
	convey.Convey("Test RecvMMsg", t, func() {
		convey.Convey("receives a batch", func() {
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
		})
		convey.Convey("returns socket errors", func() {
			container := NewMMsgContainer(1, 1)
			count, errno := RecvMMsg(-1, container)
			require.Zero(t, count)
			require.Equal(t, unix.EBADF, errno)
		})
		convey.Convey("returns an invalid descriptor error", func() {
			count, errno := RecvMMsg(-1, NewMMsgContainer(1, 1))
			convey.So(count, convey.ShouldEqual, 0)
			convey.So(errno, convey.ShouldEqual, unix.EBADF)
		})
	})
}

func TestSendMMsgConvey(t *testing.T) {
	convey.Convey("Test SendMMsg", t, func() {
		convey.Convey("sends a prepared range", func() {
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
		})
		convey.Convey("validates range and messages", func() {
			container := NewSendMMsgContainer(2)

			tests := []struct {
				start int
				end   int
			}{
				{start: -1, end: 0},
				{start: 1, end: 0},
				{start: 0, end: len(container.buffers) + 1},
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
			require.ErrorIs(t, SendMsg(-1, container, len(container.buffers)), unix.EINVAL)

			container.Prepare(0, []byte("invalid address"), SockaddrKey{})
			require.ErrorIs(t, SendMsg(-1, container, 0), unix.EINVAL)

			validKey := sockaddrKeyFromNameBuffer(t, &unix.SockaddrInet4{
				Addr: [4]byte{127, 0, 0, 1},
				Port: 9,
			})
			container.Prepare(0, []byte("closed socket"), validKey)
			require.ErrorIs(t, SendMsg(-1, container, 0), unix.EBADF)
		})
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
			err := SendMsg(-1, NewSendMMsgContainer(1), -1)
			convey.So(err, convey.ShouldEqual, unix.EINVAL)
		})
	})
}

func TestSendPreparedMsgConvey(t *testing.T) {
	convey.Convey("Test sendPreparedMsg", t, func() {
		convey.Convey("rejects an empty address key", func() {
			container := NewSendMMsgContainer(1)
			container.Prepare(0, []byte("payload"), SockaddrKey{})
			convey.So(sendPreparedMsg(-1, container, 0), convey.ShouldEqual, unix.EINVAL)
		})
	})
}

func TestAsErrnoConvey(t *testing.T) {
	convey.Convey("Test asErrno", t, func() {
		convey.Convey("normalizes Darwin errors", func() {
			require.Equal(t, unix.EINVAL, asErrno(unix.EINVAL))
			require.Equal(t, unix.EIO, asErrno(errors.New("not an errno")))
		})
		convey.Convey("preserves errno and normalizes other errors", func() {
			convey.So(asErrno(unix.EINVAL), convey.ShouldEqual, unix.EINVAL)
			convey.So(asErrno(errors.New("mock")), convey.ShouldEqual, unix.EIO)
		})
	})
}
