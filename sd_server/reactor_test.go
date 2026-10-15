package sd_server

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_session"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/sirupsen/logrus"
	"github.com/smartystreets/goconvey/convey"
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
	reactor *reactor,
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

func TestNewReactorConvey(t *testing.T) {
	convey.Convey("Test newReactor", t, func() {
		convey.Convey("initializes reactor-local resources", func() {
			selector := &conveySelector{}
			patches := gomonkey.NewPatches()
			patches.ApplyFuncReturn(sd_socket.NewSelector, selector, nil)
			patches.ApplyFuncReturn(sd_socket.UDPBoundSocket, 10, nil)
			defer patches.Reset()
			config := &sd_config.Config{
				Server:  &sd_config.ServerConfig{EventSize: 2, BatchSize: 1, BufSize: 1},
				Session: &sd_config.SessionConfig{TimeoutSec: 10},
			}
			got, err := newReactor(1, config, &unix.SockaddrInet4{}, nil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(got.id, convey.ShouldEqual, 1)
			convey.So(got.listenFD, convey.ShouldEqual, 10)
			convey.So(selector.added, convey.ShouldResemble, []int{10})
		})
	})
}

func TestReactorServeConvey(t *testing.T) {
	convey.Convey("Test reactor.serve", t, func() {
		convey.Convey("delegates polling", func() {
			r := newConveyReactor(nil)
			r.listenFD = -1
			convey.So(r.serve(context.Background()), convey.ShouldBeNil)
		})
	})
}

func TestReactorNowSecConvey(t *testing.T) {
	convey.Convey("Test reactor.nowSec", t, func() {
		convey.Convey("uses the monotonic origin", func() {
			r := &reactor{clockStart: time.Now().Add(-2 * time.Second)}
			convey.So(r.nowSec(), convey.ShouldBeGreaterThanOrEqualTo, 1)
		})
	})
}

func TestReactorEnqueueReadyConvey(t *testing.T) {
	convey.Convey("Test reactor.enqueueReady", t, func() {
		convey.Convey("deduplicates descriptors", func() {
			r := newConveyReactor(nil)
			r.enqueueReady(r.listenFD)
			r.enqueueReady(r.listenFD)
			convey.So(r.readyFDs, convey.ShouldResemble, []int{r.listenFD})
		})
	})
}

func TestReactorProcessReadyConvey(t *testing.T) {
	convey.Convey("Test reactor.processReady", t, func() {
		convey.Convey("drains non-retry descriptors", func() {
			r := newConveyReactor(nil)
			r.readyFDs = []int{r.listenFD}
			r.readySet[r.listenFD] = struct{}{}
			patches := gomonkey.ApplyFuncReturn(sd_socket.RecvMMsg, 0, unix.EAGAIN)
			defer patches.Reset()
			r.processReady()
			convey.So(r.readyFDs, convey.ShouldBeEmpty)
			convey.So(r.readySet, convey.ShouldBeEmpty)
		})
	})
}

func TestReactorHandleUploadConvey(t *testing.T) {
	convey.Convey("Test reactor.handleUpload", t, func() {
		convey.Convey("stops on EAGAIN", func() {
			r := newConveyReactor(nil)
			patches := gomonkey.ApplyFuncReturn(sd_socket.RecvMMsg, 0, unix.EAGAIN)
			defer patches.Reset()
			convey.So(r.handleUpload(), convey.ShouldBeFalse)
		})
	})
}

func TestReactorHandleUploadPacketConvey(t *testing.T) {
	convey.Convey("Test reactor.handleUploadPacket", t, func() {
		convey.Convey("creates a dropped session", func() {
			manager := newTestUploadManager(nil, sd_route.NewRouter("", nil, false))
			r := newConveyReactor(manager)
			patches := gomonkey.NewPatches()
			patches.ApplyMethodReturn(r.rxMC, "GetLengthOfMsg", uint32(1))
			patches.ApplyMethodReturn(r.rxMC, "GetBufOfMsg", []byte{1})
			patches.ApplyMethodReturn(r.rxMC, "GetRSockaddrKeyOfMsg", sd_socket.SockaddrKey{})
			defer patches.Reset()
			r.handleUploadPacket(0, 0)
			convey.So(r.sessions, convey.ShouldHaveLength, 1)
		})
	})
}

func TestReactorCreateSessionConvey(t *testing.T) {
	convey.Convey("Test reactor.createSession", t, func() {
		convey.Convey("stores and schedules a session", func() {
			r := newConveyReactor(nil)
			key := sd_socket.SockaddrKey{}
			sess := r.createSession(key)
			convey.So(r.sessions[key], convey.ShouldEqual, sess)
		})
	})
}

func TestReactorGetOrCreateSessionFDConvey(t *testing.T) {
	convey.Convey("Test reactor.getOrCreateSessionFD", t, func() {
		convey.Convey("creates and indexes a descriptor", func() {
			r := newConveyReactor(nil)
			sess := sd_session.NewSession(sd_socket.SockaddrKey{})
			patches := gomonkey.ApplyFuncReturn(sd_socket.UDPSocket, 11, nil)
			defer patches.Reset()
			fd, err := r.getOrCreateSessionFD(sess, unix.AF_INET)
			convey.So(err, convey.ShouldBeNil)
			convey.So(fd, convey.ShouldEqual, 11)
			convey.So(r.sessionsByFD[11], convey.ShouldEqual, sess)
		})
	})
}

func TestReactorHandleDownloadConvey(t *testing.T) {
	convey.Convey("Test reactor.handleDownload", t, func() {
		convey.Convey("stops on EAGAIN", func() {
			r := newConveyReactor(nil)
			patches := gomonkey.ApplyFuncReturn(sd_socket.RecvMMsg, 0, unix.EAGAIN)
			defer patches.Reset()
			convey.So(r.handleDownload(1, sd_session.NewSession(sd_socket.SockaddrKey{})), convey.ShouldBeFalse)
		})
	})
}

func TestReactorSendDownloadBatchConvey(t *testing.T) {
	convey.Convey("Test reactor.sendDownloadBatch", t, func() {
		convey.Convey("advances sent packets", func() {
			r := newConveyReactor(nil)
			patches := gomonkey.ApplyFuncReturn(sd_socket.SendMMsg, 2, nil)
			defer patches.Reset()
			convey.So(r.sendDownloadBatch(2), convey.ShouldBeNil)
		})
	})
}

func TestReactorExpireSessionsConvey(t *testing.T) {
	convey.Convey("Test reactor.expireSessions", t, func() {
		convey.Convey("removes expired sessions", func() {
			r := newConveyReactor(nil)
			key := sd_socket.SockaddrKey{}
			r.createSession(key)
			r.expireSessions(11)
			convey.So(r.sessions, convey.ShouldBeEmpty)
		})
	})
}

func TestReactorRemoveSessionConvey(t *testing.T) {
	convey.Convey("Test reactor.removeSession", t, func() {
		convey.Convey("removes it from tables", func() {
			r := newConveyReactor(nil)
			key := sd_socket.SockaddrKey{}
			sess := r.createSession(key)
			r.removeSession(sess)
			convey.So(r.sessions, convey.ShouldBeEmpty)
		})
	})
}

func TestReactorCloseSessionConvey(t *testing.T) {
	convey.Convey("Test reactor.closeSession", t, func() {
		convey.Convey("removes the session key", func() {
			r := newConveyReactor(nil)
			key := sd_socket.SockaddrKey{}
			sess := sd_session.NewSession(key)
			r.sessions[key] = sess
			r.closeSession(sess)
			convey.So(r.sessions, convey.ShouldBeEmpty)
		})
	})
}

func TestReactorCloseConvey(t *testing.T) {
	convey.Convey("Test reactor.close", t, func() {
		convey.Convey("releases selector ownership", func() {
			selector := &conveySelector{}
			r := newConveyReactor(nil)
			r.selector = selector
			patches := gomonkey.ApplyFuncReturn(unix.Close, nil)
			defer patches.Reset()
			r.close()
			convey.So(selector.deleted, convey.ShouldResemble, []int{r.listenFD})
		})
	})
}

type conveySelector struct {
	added   []int
	deleted []int
	wakes   int
	pollErr error
}

func (s *conveySelector) Add(fd int, _ sd_socket.SelectorEvent) error {
	s.added = append(s.added, fd)
	return nil
}

func (s *conveySelector) Mod(int, sd_socket.SelectorEvent) error {
	return nil
}

func (s *conveySelector) Del(fd int) error {
	s.deleted = append(s.deleted, fd)
	return nil
}

func (s *conveySelector) Wake() error {
	s.wakes++
	return nil
}

func (s *conveySelector) Polling(
	ctx context.Context,
	_ int,
	handle func([]sd_socket.PolledEvent),
) error {
	handle(nil)
	return s.pollErr
}

func (s *conveySelector) Close() error {
	return nil
}

func newConveyReactor(manager *uploadManager) *reactor {
	return &reactor{
		config: &sd_config.Config{
			Server:  &sd_config.ServerConfig{MaxTryTimes: 1},
			Session: &sd_config.SessionConfig{TimeoutSec: 10},
		},
		selector:       &conveySelector{},
		listenFD:       -1,
		sessions:       make(map[sd_socket.SockaddrKey]*sd_session.Session),
		sessionsByFD:   make(map[int]*sd_session.Session),
		sessionWheel:   sd_session.NewWheel(1, 10, 0),
		upstreamMgr:    manager,
		rxMC:           sd_socket.NewMMsgContainer(1, 1),
		txMC:           sd_socket.NewSendMMsgContainer(1),
		readySet:       make(map[int]struct{}),
		recvBatchLimit: 1,
		clockStart:     time.Now(),
		logger:         logrus.New().WithField("test", true),
	}
}
