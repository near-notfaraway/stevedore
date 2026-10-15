package sd_upstream

import (
	"net"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func mustNewPeer(t testing.TB, id int, addr string, config *sd_config.PeerConfig) *Peer {
	t.Helper()
	peer, err := NewPeer(id, addr, config)
	require.NoError(t, err)
	return peer
}

func TestNewPeer(t *testing.T) {
	peer := mustNewPeer(t, 2, "127.0.0.1:2345", &sd_config.PeerConfig{Weight: 3})

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
			peer := mustNewPeer(t, 0, tt.addr, &sd_config.PeerConfig{Weight: 1})
			familyPeer, ok := any(peer).(interface{ GetFamily() int })

			require.True(t, ok)
			require.Equal(t, tt.family, familyPeer.GetFamily())
		})
	}
}

func TestNewPeerRejectsInvalidAddress(t *testing.T) {
	peer, err := NewPeer(0, "invalid address", &sd_config.PeerConfig{Weight: 1})

	require.Nil(t, peer)
	require.EqualError(t, err, "resolve peer addr invalid address failed")
}

func TestPeerState(t *testing.T) {
	peer := testPeer("127.0.0.1:2345", 1)

	peer.setState(peerDead)
	require.False(t, peer.isAlive())

	peer.setState(peerAlive)
	require.True(t, peer.isAlive())
}

func TestPeerSend(t *testing.T) {
	receiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer receiver.Close()

	fd, err := sd_socket.UDPSocket(unix.AF_INET, false, false, false)
	require.NoError(t, err)
	defer unix.Close(fd)

	peer := mustNewPeer(t, 0, receiver.LocalAddr().String(), &sd_config.PeerConfig{Weight: 1})
	payload := []byte("payload")
	require.NoError(t, peer.Send(fd, payload))

	require.NoError(t, receiver.SetReadDeadline(time.Now().Add(time.Second)))
	buf := make([]byte, len(payload))
	n, _, err := receiver.ReadFromUDP(buf)
	require.NoError(t, err)
	require.Equal(t, payload, buf[:n])
}

func TestNewPeerConvey(t *testing.T) {
	convey.Convey("Test NewPeer", t, func() {
		convey.Convey("resolves address and initializes state", func() {
			peerConfig := &sd_config.PeerConfig{Weight: 1}
			patches := gomonkey.ApplyFuncReturn(
				sd_socket.ResolveUDPSockaddr,
				&unix.SockaddrInet6{},
			)
			defer patches.Reset()
			peer := mustNewPeer(t, 2, "[::1]:10001", peerConfig)
			convey.So(peer.id, convey.ShouldEqual, 2)
			convey.So(peer.GetFamily(), convey.ShouldEqual, unix.AF_INET6)
			convey.So(peer.isAlive(), convey.ShouldBeTrue)
		})
	})
}

func TestPeerSendConvey(t *testing.T) {
	convey.Convey("Test Peer.Send", t, func() {
		convey.Convey("delegates to the socket package", func() {
			called := false
			patches := gomonkey.ApplyFunc(
				sd_socket.SendTo,
				func(fd int, data []byte, flags int, to unix.Sockaddr) error {
					called = true
					convey.So(fd, convey.ShouldEqual, 3)
					convey.So(data, convey.ShouldResemble, []byte("payload"))
					return nil
				},
			)
			defer patches.Reset()
			peer := &Peer{sockaddr: &unix.SockaddrInet4{}}
			convey.So(peer.Send(3, []byte("payload")), convey.ShouldBeNil)
			convey.So(called, convey.ShouldBeTrue)
		})
	})
}

func TestPeerSetStateConvey(t *testing.T) {
	convey.Convey("Test Peer.setState", t, func() {
		convey.Convey("stores the requested state", func() {
			peer := &Peer{}
			peer.setState(peerAlive)
			convey.So(peerState(peer.state), convey.ShouldEqual, peerAlive)
		})
	})
}

func TestPeerIsAliveConvey(t *testing.T) {
	convey.Convey("Test Peer.isAlive", t, func() {
		convey.Convey("checks the atomic state", func() {
			peer := &Peer{state: int32(peerDead)}
			convey.So(peer.isAlive(), convey.ShouldBeFalse)
			peer.state = int32(peerAlive)
			convey.So(peer.isAlive(), convey.ShouldBeTrue)
		})
	})
}

func TestPeerGetAddrConvey(t *testing.T) {
	convey.Convey("Test Peer.GetAddr", t, func() {
		convey.Convey("returns the configured address", func() {
			convey.So((&Peer{addr: "127.0.0.1:1"}).GetAddr(), convey.ShouldEqual, "127.0.0.1:1")
		})
	})
}

func TestPeerGetFamilyConvey(t *testing.T) {
	convey.Convey("Test Peer.GetFamily", t, func() {
		convey.Convey("returns the resolved family", func() {
			convey.So((&Peer{family: unix.AF_INET}).GetFamily(), convey.ShouldEqual, unix.AF_INET)
		})
	})
}
