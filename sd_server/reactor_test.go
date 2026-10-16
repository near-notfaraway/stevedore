package sd_server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_session"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/sirupsen/logrus"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

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

func TestReactorUpdateUploadManagerConvey(t *testing.T) {
	convey.Convey("Test reactor.updateUploadManager", t, func() {
		convey.Convey("runs the update in the event loop", func() {
			oldManager := newTestUploadManager(nil, mustNewRouter(t, "old", nil, false))
			newManager := newTestUploadManager(nil, mustNewRouter(t, "new", nil, false))
			selector := &commandSelector{wakeCh: make(chan struct{}, 1)}
			r := newConveyReactor(oldManager)
			r.selector = selector
			patches := gomonkey.ApplyFuncReturn(unix.Close, nil)
			defer patches.Reset()

			serveDone := make(chan error, 1)
			go func() {
				serveDone <- r.serve(context.Background())
			}()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			require.NoError(t, r.updateUploadManager(ctx, newManager))
			require.NoError(t, <-serveDone)
			require.Same(t, newManager, r.uploadMgr)
			require.Equal(t, 1, selector.wakes)
		})
		convey.Convey("skips a cancelled command", func() {
			oldManager := newTestUploadManager(nil, mustNewRouter(t, "old", nil, false))
			newManager := newTestUploadManager(nil, mustNewRouter(t, "new", nil, false))
			r := newConveyReactor(oldManager)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			require.ErrorIs(t, r.updateUploadManager(ctx, newManager), context.DeadlineExceeded)

			r.processCommands()

			require.Same(t, oldManager, r.uploadMgr)
		})
		convey.Convey("skips the command after wake failure", func() {
			oldManager := newTestUploadManager(nil, mustNewRouter(t, "old", nil, false))
			newManager := newTestUploadManager(nil, mustNewRouter(t, "new", nil, false))
			r := newConveyReactor(oldManager)
			r.selector = &conveySelector{wakeErr: errors.New("wake failed")}

			err := r.updateUploadManager(context.Background(), newManager)
			require.Error(t, err)

			r.processCommands()

			require.Same(t, oldManager, r.uploadMgr)
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
			manager := newTestUploadManager(nil, mustNewRouter(t, "", nil, false))
			r := newConveyReactor(manager)
			patches := gomonkey.NewPatches()
			patches.ApplyMethodReturn(r.rxContainer, "GetLengthOfMsg", uint32(1))
			patches.ApplyMethodReturn(r.rxContainer, "GetBufOfMsg", []byte{1})
			patches.ApplyMethodReturn(r.rxContainer, "GetRSockaddrKeyOfMsg", sd_socket.SockaddrKey{})
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
		convey.Convey("manages descriptors by address family", func() {
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
		convey.Convey("forwards an IPv6 response", func() {
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

			reactor.rxContainer.ResetNameLengths()
			packetCount := 0
			receiveDeadline := time.Now().Add(time.Second)
			for {
				count, errno := sd_socket.RecvMMsg(reactor.listenFD, reactor.rxContainer)
				if count > 0 {
					require.Zero(t, errno)
					packetCount = count
					break
				}
				if errno != unix.EAGAIN && errno != unix.EWOULDBLOCK {
					require.FailNow(t, "receive batch failed", "errno: %v", errno)
				}
				if time.Now().After(receiveDeadline) {
					require.FailNow(t, "receive batch timed out")
				}
				time.Sleep(time.Millisecond)
			}
			require.Equal(t, 1, packetCount)
			require.Equal(t, upload, reactor.rxContainer.GetBufOfMsg(0)[:reactor.rxContainer.GetLengthOfMsg(0)])

			key := reactor.rxContainer.GetRSockaddrKeyOfMsg(0)
			sess := reactor.createSession(key)
			peer := mustNewPeer(t, 0, backend.LocalAddr().String(), &sd_config.PeerConfig{Weight: 1})
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
			downloadDeadline := time.Now().Add(time.Second)
			for {
				reactor.handleDownload(sessionFD, sess)
				readDeadline := time.Now().Add(10 * time.Millisecond)
				if readDeadline.After(downloadDeadline) {
					readDeadline = downloadDeadline
				}
				require.NoError(t, client.SetReadDeadline(readDeadline))
				n, _, err = client.ReadFromUDP(clientBuf)
				if err == nil {
					break
				}
				if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
					require.NoError(t, err)
				}
				if time.Now().After(downloadDeadline) {
					require.FailNow(t, "receive from client timed out")
				}
			}
			require.Equal(t, download, clientBuf[:n])
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
	wakeErr error
	pollErr error
}

type commandSelector struct {
	wakeCh chan struct{}
	wakes  int
}

type loopCommandSelector struct {
	wakeCh    chan struct{}
	wakeErr   error
	wakeCount int
	failAt    int
}

func (s *commandSelector) Add(int, sd_socket.SelectorEvent) error {
	return nil
}

func (s *commandSelector) Mod(int, sd_socket.SelectorEvent) error {
	return nil
}

func (s *commandSelector) Del(int) error {
	return nil
}

func (s *commandSelector) Wake() error {
	s.wakes++
	s.wakeCh <- struct{}{}
	return nil
}

func (s *commandSelector) Polling(
	ctx context.Context,
	_ int,
	handle func([]sd_socket.PolledEvent),
) error {
	select {
	case <-s.wakeCh:
		handle(nil)
	case <-ctx.Done():
	}
	return nil
}

func (s *commandSelector) Close() error {
	return nil
}

func (s *loopCommandSelector) Add(int, sd_socket.SelectorEvent) error {
	return nil
}

func (s *loopCommandSelector) Mod(int, sd_socket.SelectorEvent) error {
	return nil
}

func (s *loopCommandSelector) Del(int) error {
	return nil
}

func (s *loopCommandSelector) Wake() error {
	s.wakeCount++
	if s.wakeErr != nil && (s.failAt == 0 || s.wakeCount >= s.failAt) {
		return s.wakeErr
	}
	s.wakeCh <- struct{}{}
	return nil
}

func (s *loopCommandSelector) Polling(
	ctx context.Context,
	_ int,
	handle func([]sd_socket.PolledEvent),
) error {
	for {
		select {
		case <-s.wakeCh:
			handle(nil)
		case <-ctx.Done():
			return nil
		}
	}
}

func (s *loopCommandSelector) Close() error {
	return nil
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
	return s.wakeErr
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

func newConveyReactor(uploadMgr *uploadManager) *reactor {
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
		uploadMgr:      uploadMgr,
		rxContainer:    sd_socket.NewMMsgContainer(1, 1),
		txContainer:    sd_socket.NewSendMMsgContainer(1),
		readySet:       make(map[int]struct{}),
		recvBatchLimit: 1,
		clockStart:     time.Now(),
		logger:         logrus.New().WithField("test", true),
		commandCh:      make(chan *reactorCommand, reactorCommandQueueSize),
	}
}
