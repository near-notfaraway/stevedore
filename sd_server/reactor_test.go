package sd_server

import (
	"net"
	"testing"
	"time"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_session"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestReactorManagesSessionFDsByAddressFamily(t *testing.T) {
	config := &sd_config.Config{
		Server: &sd_config.ServerConfig{
			EventSize: 8,
			BatchSize: 32,
			BufSize:   4096,
		},
		Session: &sd_config.SessionConfig{
			RecycleIntervalSec: 1,
			TimeoutSec:         30,
		},
	}
	reactor, err := newReactor(0, config, &unix.SockaddrInet4{
		Addr: [4]byte{127, 0, 0, 1},
	}, nil)
	require.NoError(t, err)
	defer reactor.close()

	key := sd_socket.SockaddrKey{}
	sess := reactor.createSession(key)
	require.Empty(t, sess.GetFDs())

	fdProvider, ok := any(reactor).(interface {
		getOrCreateSessionFD(*sd_session.Session, int) (int, error)
	})
	require.True(t, ok)

	ipv4FD, err := fdProvider.getOrCreateSessionFD(sess, unix.AF_INET)
	require.NoError(t, err)
	reusedIPv4FD, err := fdProvider.getOrCreateSessionFD(sess, unix.AF_INET)
	require.NoError(t, err)
	require.Equal(t, ipv4FD, reusedIPv4FD)

	ipv6FD, err := fdProvider.getOrCreateSessionFD(sess, unix.AF_INET6)
	require.NoError(t, err)
	require.NotEqual(t, ipv4FD, ipv6FD)
	require.Same(t, sess, reactor.sessionsByFD[ipv4FD])
	require.Same(t, sess, reactor.sessionsByFD[ipv6FD])

	ipv4Addr, err := unix.Getsockname(ipv4FD)
	require.NoError(t, err)
	require.IsType(t, &unix.SockaddrInet4{}, ipv4Addr)
	ipv6Addr, err := unix.Getsockname(ipv6FD)
	require.NoError(t, err)
	require.IsType(t, &unix.SockaddrInet6{}, ipv6Addr)

	reactor.removeSession(sess)

	require.NotContains(t, reactor.sessions, key)
	require.NotContains(t, reactor.sessionsByFD, ipv4FD)
	require.NotContains(t, reactor.sessionsByFD, ipv6FD)
	_, err = unix.Getsockname(ipv4FD)
	require.Error(t, err)
	_, err = unix.Getsockname(ipv6FD)
	require.Error(t, err)
}

func TestReactorForwardsIPv6PacketAndResponse(t *testing.T) {
	config := &sd_config.Config{
		Server: &sd_config.ServerConfig{
			EventSize:           8,
			BatchSize:           32,
			BufSize:             4096,
			MaxRecvBatchesPerFD: 1,
		},
		Session: &sd_config.SessionConfig{
			RecycleIntervalSec: 1,
			TimeoutSec:         30,
		},
	}
	reactor, err := newReactor(0, config, &unix.SockaddrInet6{
		Addr: [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
	}, nil)
	require.NoError(t, err)
	defer reactor.close()

	listenSockaddr, err := unix.Getsockname(reactor.listenFD)
	require.NoError(t, err)
	listenAddr := sd_socket.SockaddrToUDPAddr(listenSockaddr)
	require.NotNil(t, listenAddr)

	client, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback})
	require.NoError(t, err)
	defer client.Close()
	backend, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback})
	require.NoError(t, err)
	defer backend.Close()

	upload := []byte("ipv6 upload")
	_, err = client.WriteToUDP(upload, listenAddr)
	require.NoError(t, err)

	reactor.rxMC.ResetNameLengths()
	packetCount := receiveBatch(t, reactor.listenFD, reactor.rxMC)
	require.Equal(t, 1, packetCount)
	require.Equal(t, upload, reactor.rxMC.GetBufOfMsg(0)[:reactor.rxMC.GetLengthOfMsg(0)])

	key := reactor.rxMC.GetRSockaddrKeyOfMsg(0)
	sess := reactor.createSession(key)
	peer := sd_upstream.NewPeer(0, backend.LocalAddr().String(), &sd_config.PeerConfig{Weight: 1})
	sessionFD, err := reactor.getOrCreateSessionFD(sess, peer.GetFamily())
	require.NoError(t, err)
	require.NoError(t, peer.Send(sessionFD, upload))

	require.NoError(t, backend.SetReadDeadline(time.Now().Add(time.Second)))
	backendBuf := make([]byte, 128)
	n, sessionAddr, err := backend.ReadFromUDP(backendBuf)
	require.NoError(t, err)
	require.Equal(t, upload, backendBuf[:n])

	download := []byte("ipv6 download")
	_, err = backend.WriteToUDP(download, sessionAddr)
	require.NoError(t, err)

	clientBuf := make([]byte, 128)
	n = receiveFromClient(t, reactor, sessionFD, sess, client, clientBuf)
	require.Equal(t, download, clientBuf[:n])
}

func receiveBatch(t *testing.T, fd int, container *sd_socket.MMsgContainer) int {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		n, errno := sd_socket.RecvMMsg(fd, container)
		if n > 0 {
			require.Zero(t, errno)
			return n
		}
		if errno != unix.EAGAIN && errno != unix.EWOULDBLOCK {
			require.FailNow(t, "receive batch failed", "errno: %v", errno)
		}
		if time.Now().After(deadline) {
			require.FailNow(t, "receive batch timed out")
		}
		time.Sleep(time.Millisecond)
	}
}

func receiveFromClient(
	t *testing.T,
	reactor *Reactor,
	sessionFD int,
	sess *sd_session.Session,
	client *net.UDPConn,
	buf []byte,
) int {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		reactor.handleDownload(sessionFD, sess)
		readDeadline := time.Now().Add(10 * time.Millisecond)
		if readDeadline.After(deadline) {
			readDeadline = deadline
		}
		require.NoError(t, client.SetReadDeadline(readDeadline))
		n, _, err := client.ReadFromUDP(buf)
		if err == nil {
			return n
		}
		if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
			require.NoError(t, err)
		}
		if time.Now().After(deadline) {
			require.FailNow(t, "receive from client timed out")
		}
	}
}
