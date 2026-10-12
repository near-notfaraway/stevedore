package sd_upstream

import (
	"net"
	"testing"
	"time"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestNewPeer(t *testing.T) {
	peer := NewPeer(2, "127.0.0.1:2345", &sd_config.PeerConfig{Weight: 3})

	require.Equal(t, 2, peer.id)
	require.Equal(t, "127.0.0.1:2345", peer.GetAddr())
	require.Equal(t, 3, peer.weight)
	require.IsType(t, &unix.SockaddrInet4{}, peer.sockaddr)
	require.True(t, peer.isAlive())
}

func TestPeerReportsAddressFamily(t *testing.T) {
	tests := []struct {
		name   string
		addr   string
		family int
	}{
		{name: "IPv4", addr: "127.0.0.1:2345", family: unix.AF_INET},
		{name: "IPv6", addr: "[2001:db8::10]:2345", family: unix.AF_INET6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer := NewPeer(0, tt.addr, &sd_config.PeerConfig{Weight: 1})
			familyPeer, ok := any(peer).(interface{ GetFamily() int })

			require.True(t, ok)
			require.Equal(t, tt.family, familyPeer.GetFamily())
		})
	}
}

func TestNewPeerRejectsInvalidAddress(t *testing.T) {
	require.Panics(t, func() {
		NewPeer(0, "invalid address", &sd_config.PeerConfig{Weight: 1})
	})
}

func TestPeerState(t *testing.T) {
	peer := testPeer("127.0.0.1:2345", 1)

	peer.SetState(PeerDead)
	require.False(t, peer.isAlive())

	peer.SetState(PeerAlive)
	require.True(t, peer.isAlive())
}

func TestPeerSend(t *testing.T) {
	receiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer receiver.Close()

	fd, err := sd_socket.UDPSocket(unix.AF_INET, false, false, false)
	require.NoError(t, err)
	defer unix.Close(fd)

	peer := NewPeer(0, receiver.LocalAddr().String(), &sd_config.PeerConfig{Weight: 1})
	payload := []byte("payload")
	require.NoError(t, peer.Send(fd, payload))

	require.NoError(t, receiver.SetReadDeadline(time.Now().Add(time.Second)))
	buf := make([]byte, len(payload))
	n, _, err := receiver.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, payload, buf[:n])
}
