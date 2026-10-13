package sd_socket

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestUDPBoundSocket(t *testing.T) {
	t.Run("rejects unsupported address", func(t *testing.T) {
		fd, err := UDPBoundSocket(&unix.SockaddrUnix{Name: "/tmp/stevedore.sock"}, false, false, false)
		require.Equal(t, -1, fd)
		require.EqualError(t, err, "sockaddr family is not inet")
	})

	t.Run("binds IPv4 socket", func(t *testing.T) {
		fd, err := UDPBoundSocket(&unix.SockaddrInet4{
			Addr: [4]byte{127, 0, 0, 1},
		}, false, false, false)
		require.NoError(t, err)
		t.Cleanup(func() { _ = unix.Close(fd) })

		addr, err := unix.Getsockname(fd)
		require.NoError(t, err)
		boundAddr, ok := addr.(*unix.SockaddrInet4)
		require.True(t, ok)
		require.Equal(t, [4]byte{127, 0, 0, 1}, boundAddr.Addr)
		require.NotZero(t, boundAddr.Port)
	})

	t.Run("binds IPv6 socket", func(t *testing.T) {
		fd, err := UDPBoundSocket(&unix.SockaddrInet6{
			Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		}, false, false, false)
		require.NoError(t, err)
		t.Cleanup(func() { _ = unix.Close(fd) })

		addr, err := unix.Getsockname(fd)
		require.NoError(t, err)
		boundAddr, ok := addr.(*unix.SockaddrInet6)
		require.True(t, ok)
		require.Equal(t, [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}, boundAddr.Addr)
		require.NotZero(t, boundAddr.Port)
	})

	t.Run("returns bind error", func(t *testing.T) {
		firstFD, err := UDPBoundSocket(&unix.SockaddrInet4{
			Addr: [4]byte{127, 0, 0, 1},
		}, false, false, false)
		require.NoError(t, err)
		t.Cleanup(func() { _ = unix.Close(firstFD) })

		addr, err := unix.Getsockname(firstFD)
		require.NoError(t, err)

		fd, err := UDPBoundSocket(addr, false, false, false)
		require.Equal(t, -1, fd)
		require.Error(t, err)
		require.Contains(t, err.Error(), "bind sockaddr failed")
	})
}

func TestUDPSocket(t *testing.T) {
	t.Run("rejects unsupported family", func(t *testing.T) {
		fd, err := UDPSocket(unix.AF_UNIX, false, false, false)
		require.Equal(t, -1, fd)
		require.EqualError(t, err, "family is not inet")
	})

	t.Run("creates IPv6 socket", func(t *testing.T) {
		fd, err := UDPSocket(unix.AF_INET6, false, false, false)
		require.NoError(t, err)
		require.NoError(t, unix.Close(fd))
	})

	tests := []struct {
		name      string
		nonblock  bool
		reuseAddr bool
		reusePort bool
	}{
		{name: "blocking", nonblock: false},
		{name: "nonblocking with reuse", nonblock: true, reuseAddr: true, reusePort: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fd, err := UDPSocket(unix.AF_INET, tt.nonblock, tt.reuseAddr, tt.reusePort)
			require.NoError(t, err)
			t.Cleanup(func() { _ = unix.Close(fd) })

			statusFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
			require.NoError(t, err)
			require.Equal(t, tt.nonblock, statusFlags&unix.O_NONBLOCK != 0)

			descriptorFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
			require.NoError(t, err)
			require.NotZero(t, descriptorFlags&unix.FD_CLOEXEC)

			if tt.reuseAddr {
				value, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR)
				require.NoError(t, err)
				require.NotZero(t, value)
			}
			if tt.reusePort {
				value, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT)
				require.NoError(t, err)
				require.NotZero(t, value)
			}
		})
	}
}

func TestSendTo(t *testing.T) {
	t.Run("sends UDP datagram", func(t *testing.T) {
		receiverFD, err := UDPBoundSocket(&unix.SockaddrInet4{
			Addr: [4]byte{127, 0, 0, 1},
		}, false, false, false)
		require.NoError(t, err)
		t.Cleanup(func() { _ = unix.Close(receiverFD) })
		require.NoError(t, unix.SetsockoptTimeval(
			receiverFD,
			unix.SOL_SOCKET,
			unix.SO_RCVTIMEO,
			&unix.Timeval{Sec: 1},
		))

		receiverAddr, err := unix.Getsockname(receiverFD)
		require.NoError(t, err)

		senderFD, err := UDPSocket(unix.AF_INET, false, false, false)
		require.NoError(t, err)
		t.Cleanup(func() { _ = unix.Close(senderFD) })

		payload := []byte("stevedore")
		require.NoError(t, SendTo(senderFD, payload, 0, receiverAddr))

		buf := make([]byte, 64)
		n, _, err := unix.Recvfrom(receiverFD, buf, 0)
		require.NoError(t, err)
		require.Equal(t, payload, buf[:n])
	})

	t.Run("returns send error", func(t *testing.T) {
		err := SendTo(-1, []byte("payload"), 0, &unix.SockaddrInet4{
			Addr: [4]byte{127, 0, 0, 1},
			Port: 1,
		})
		require.Error(t, err)
	})
}

func TestUDPBoundSocketConvey(t *testing.T) {
	convey.Convey("Test UDPBoundSocket", t, func() {
		convey.Convey("rejects a non-IP sockaddr", func() {
			fd, err := UDPBoundSocket(&unix.SockaddrUnix{Name: "invalid"}, false, false, false)
			convey.So(fd, convey.ShouldEqual, -1)
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestUDPSocketConvey(t *testing.T) {
	convey.Convey("Test UDPSocket", t, func() {
		convey.Convey("delegates platform socket creation", func() {
			patches := gomonkey.ApplyFuncReturn(newUDPSocket, 99, nil)
			defer patches.Reset()
			fd, err := UDPSocket(unix.AF_INET, false, false, false)
			convey.So(err, convey.ShouldBeNil)
			convey.So(fd, convey.ShouldEqual, 99)
		})
	})
}

func TestSendToConvey(t *testing.T) {
	convey.Convey("Test SendTo", t, func() {
		convey.Convey("delegates to unix.Sendto", func() {
			called := false
			patches := gomonkey.ApplyFunc(
				unix.Sendto,
				func(fd int, p []byte, flags int, to unix.Sockaddr) error {
					called = true
					convey.So(fd, convey.ShouldEqual, 3)
					convey.So(p, convey.ShouldResemble, []byte("payload"))
					return nil
				},
			)
			defer patches.Reset()
			err := SendTo(3, []byte("payload"), 0, &unix.SockaddrInet4{})
			convey.So(err, convey.ShouldBeNil)
			convey.So(called, convey.ShouldBeTrue)
		})
	})
}
