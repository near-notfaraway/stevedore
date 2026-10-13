//go:build linux

package sd_socket

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSocketPlatformCapabilitiesLinux(t *testing.T) {
	require.True(t, ReusePortShardingSupported())

	fd, err := newUDPSocket(-1, false)
	require.Equal(t, -1, fd)
	require.Error(t, err)
}

func TestUDPSocketLinuxSetsDescriptorFlags(t *testing.T) {
	for _, nonblock := range []bool{false, true} {
		fd, err := UDPSocket(unix.AF_INET, nonblock, false, false)
		require.NoError(t, err)

		statusFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
		require.NoError(t, err)
		require.Equal(t, nonblock, statusFlags&unix.O_NONBLOCK != 0)

		descriptorFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		require.NoError(t, err)
		require.NotZero(t, descriptorFlags&unix.FD_CLOEXEC)
		require.NoError(t, unix.Close(fd))
	}
}

func TestReusePortShardingSupportedConvey(t *testing.T) {
	convey.Convey("Test ReusePortShardingSupported", t, func() {
		convey.Convey("reports Linux behavior", func() {
			convey.So(ReusePortShardingSupported(), convey.ShouldBeTrue)
		})
	})
}

func TestNewUDPSocketConvey(t *testing.T) {
	convey.Convey("Test newUDPSocket", t, func() {
		convey.Convey("creates a datagram descriptor", func() {
			fd, err := newUDPSocket(unix.AF_INET, true)
			convey.So(err, convey.ShouldBeNil)
			convey.So(fd, convey.ShouldBeGreaterThanOrEqualTo, 0)
			convey.So(unix.Close(fd), convey.ShouldBeNil)
		})
	})
}
