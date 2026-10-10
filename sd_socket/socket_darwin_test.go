//go:build darwin
// +build darwin

package sd_socket

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestSocketPlatformCapabilitiesDarwin(t *testing.T) {
	require.False(t, ReusePortShardingSupported())

	fd, err := newUDPSocket(-1, false)
	require.Equal(t, -1, fd)
	require.Error(t, err)
}

func TestUDPSocketDarwinSetsDescriptorFlags(t *testing.T) {
	tests := []struct {
		name            string
		nonblock        bool
		wantNonblocking bool
	}{
		{name: "blocking", nonblock: false, wantNonblocking: false},
		{name: "nonblocking", nonblock: true, wantNonblocking: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fd, err := UDPSocket(unix.AF_INET, tt.nonblock, false, false)
			require.NoError(t, err)
			defer unix.Close(fd)

			statusFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
			require.NoError(t, err)
			require.Equal(t, tt.wantNonblocking, statusFlags&unix.O_NONBLOCK != 0)

			descriptorFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
			require.NoError(t, err)
			require.NotZero(t, descriptorFlags&unix.FD_CLOEXEC)
		})
	}
}

func TestUDPBoundSocketClosesFDWhenBindFails(t *testing.T) {
	occupiedFD, err := UDPBoundSocket(&unix.SockaddrInet4{
		Addr: [4]byte{127, 0, 0, 1},
	}, false, false, false)
	require.NoError(t, err)
	defer unix.Close(occupiedFD)

	occupiedAddr, err := unix.Getsockname(occupiedFD)
	require.NoError(t, err)

	before := countOpenFDs()
	for i := 0; i < 32; i++ {
		fd, bindErr := UDPBoundSocket(occupiedAddr, false, false, false)
		require.Error(t, bindErr)
		require.Equal(t, -1, fd)
	}
	require.Equal(t, before, countOpenFDs())
}

func TestUDPReusePortDarwinKeepsEachFlowOnOneSocket(t *testing.T) {
	const (
		listenerCount  = 4
		flowCount      = 32
		packetsPerFlow = 8
	)

	listeners := make([]int, 0, listenerCount)
	firstFD, err := UDPBoundSocket(&unix.SockaddrInet4{
		Addr: [4]byte{127, 0, 0, 1},
	}, true, true, true)
	require.NoError(t, err)
	listeners = append(listeners, firstFD)
	t.Cleanup(func() { _ = unix.Close(firstFD) })

	listenAddr, err := unix.Getsockname(firstFD)
	require.NoError(t, err)
	for i := 1; i < listenerCount; i++ {
		fd, bindErr := UDPBoundSocket(listenAddr, true, true, true)
		require.NoError(t, bindErr)
		listeners = append(listeners, fd)
		t.Cleanup(func() { _ = unix.Close(fd) })
	}

	for flow := 0; flow < flowCount; flow++ {
		sender, socketErr := UDPSocket(unix.AF_INET, false, false, false)
		require.NoError(t, socketErr)
		require.NoError(t, unix.Bind(sender, &unix.SockaddrInet4{
			Addr: [4]byte{127, 0, 0, 1},
		}))
		for packet := 0; packet < packetsPerFlow; packet++ {
			require.NoError(t, unix.Sendto(sender, []byte{byte(flow), byte(packet)}, 0, listenAddr))
		}
		require.NoError(t, unix.Close(sender))
	}

	flowOwners := make(map[byte]int, flowCount)
	received := 0
	for listenerIndex, fd := range listeners {
		for {
			var payload [2]byte
			n, _, recvErr := unix.Recvfrom(fd, payload[:], 0)
			if recvErr == unix.EAGAIN || recvErr == unix.EWOULDBLOCK {
				break
			}
			require.NoError(t, recvErr)
			require.Equal(t, len(payload), n)

			owner, exists := flowOwners[payload[0]]
			if exists {
				require.Equal(t, owner, listenerIndex, "flow %d moved between listeners", payload[0])
			} else {
				flowOwners[payload[0]] = listenerIndex
			}
			received++
		}
	}

	require.Equal(t, flowCount*packetsPerFlow, received)
	require.Len(t, flowOwners, flowCount)
}

func countOpenFDs() int {
	count := 0
	for fd := 0; fd < 1024; fd++ {
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
			count++
		}
	}
	return count
}
