package sd_server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_session"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	reactorPollTimeoutMs    = 1000
	defaultRecvBatchesPerFD = 8
)

type Reactor struct {
	id             int                                           // unique reactor id
	config         *sd_config.Config                             // immutable global configuration
	selector       sd_socket.Selector                            // reactor-local platform selector
	listenFD       int                                           // SO_REUSEPORT listener owned by this reactor
	sessions       map[sd_socket.SockaddrKey]*sd_session.Session // downstream key to session
	sessionsByFD   map[int]*sd_session.Session                   // upstream fd to session
	sessionWheel   *sd_session.Wheel                             // reactor-local idle session time wheel
	upstreamMgr    *uploadManager                                // reactor-local upstream selectors and shared routes
	rxMC           *sd_socket.MMsgContainer                      // reactor-local shared batch receive buffers
	txMC           *sd_socket.SendMMsgContainer                  // reactor-local zero-copy batch send descriptors
	readyFDs       []int                                         // reactor-local round-robin ready queue
	readySet       map[int]struct{}                              // prevents duplicate fds in readyFDs
	recvBatchLimit int                                           // maximum recvmmsg calls per fd per round
	clockStart     time.Time                                     // monotonic origin for session expiration
	logger         *logrus.Entry                                 // reactor-scoped logger
}

func newReactor(id int, config *sd_config.Config, listenSa unix.Sockaddr, upstreamMgr *uploadManager) (*Reactor, error) {
	eventSize := config.Server.EventSize
	if eventSize < 1 {
		eventSize = sd_socket.DefaultEventSize
	}
	selector, err := sd_socket.NewSelector(eventSize, true)
	if err != nil {
		return nil, fmt.Errorf("create selector: %w", err)
	}

	listenFD, err := sd_socket.UDPBoundSocket(listenSa, true, true, true)
	if err != nil {
		_ = selector.Close()
		return nil, fmt.Errorf("create listen socket: %w", err)
	}
	if err = selector.Add(listenFD, sd_socket.SelectorEventRead); err != nil {
		_ = unix.Close(listenFD)
		_ = selector.Close()
		return nil, fmt.Errorf("add listen socket to selector: %w", err)
	}

	recycleInterval := config.Session.RecycleIntervalSec
	if recycleInterval < 1 {
		recycleInterval = 1
	}
	recvBatchLimit := config.Server.MaxRecvBatchesPerFD
	if recvBatchLimit < 1 {
		recvBatchLimit = defaultRecvBatchesPerFD
	}
	clockStart := time.Now()
	return &Reactor{
		id:             id,
		config:         config,
		selector:       selector,
		listenFD:       listenFD,
		sessions:       make(map[sd_socket.SockaddrKey]*sd_session.Session),
		sessionsByFD:   make(map[int]*sd_session.Session),
		sessionWheel:   sd_session.NewWheel(recycleInterval, config.Session.TimeoutSec, 0),
		upstreamMgr:    upstreamMgr,
		rxMC:           sd_socket.NewMMsgContainer(config.Server.BatchSize, config.Server.BufSize),
		txMC:           sd_socket.NewSendMMsgContainer(config.Server.BatchSize),
		readyFDs:       make([]int, 0, eventSize),
		readySet:       make(map[int]struct{}),
		recvBatchLimit: recvBatchLimit,
		clockStart:     clockStart,
		logger:         logrus.WithField("reactor_id", id),
	}, nil
}

func (r *Reactor) Serve(ctx context.Context) error {
	r.logger.Debug("start reactor")
	defer r.close()

	return r.selector.Polling(ctx, reactorPollTimeoutMs, func(evs []sd_socket.PolledEvent) {
		for i := range evs {
			if evs[i].Events&sd_socket.SelectorEventRead == 0 {
				continue
			}
			r.enqueueReady(evs[i].FD)
		}
		r.processReady()
		r.expireSessions(r.nowSec())
	})
}

func (r *Reactor) nowSec() int64 {
	return int64(time.Since(r.clockStart) / time.Second)
}

func (r *Reactor) enqueueReady(fd int) {
	if fd != r.listenFD {
		if _, ok := r.sessionsByFD[fd]; !ok {
			return
		}
	}
	if _, exists := r.readySet[fd]; exists {
		return
	}
	r.readySet[fd] = struct{}{}
	r.readyFDs = append(r.readyFDs, fd)
}

func (r *Reactor) processReady() {
	current := r.readyFDs
	r.readyFDs = current[:0]

	for _, fd := range current {
		delete(r.readySet, fd)

		var needsRetry bool
		if fd == r.listenFD {
			needsRetry = r.handleUpload()
		} else if sess, ok := r.sessionsByFD[fd]; ok {
			needsRetry = r.handleDownload(fd, sess)
		}
		if needsRetry {
			r.enqueueReady(fd)
		}
	}

	if len(r.readyFDs) > 0 {
		if err := r.selector.Wake(); err != nil {
			r.logger.Errorf("wake reactor selector failed: %v", err)
		}
	}
}

func (r *Reactor) handleUpload() bool {
	r.rxMC.ResetNameLengths()
	for batch := 0; batch < r.recvBatchLimit; batch++ {
		nPkt, errno := sd_socket.RecvMMsg(r.listenFD, r.rxMC)
		if nPkt < 1 || errno != 0 {
			if errno != unix.EAGAIN && errno != unix.EWOULDBLOCK {
				r.logger.Errorf("receive upload packets failed: %s", os.NewSyscallError("recvmmsg", errno))
			}
			return false
		}

		now := r.nowSec()
		for i := 0; i < nPkt; i++ {
			r.handleUploadPacket(i, now)
		}
	}
	return true
}

func (r *Reactor) handleUploadPacket(index int, now int64) {
	nr := r.rxMC.GetLengthOfMsg(index)
	buf := r.rxMC.GetBufOfMsg(index)[:nr]
	key := r.rxMC.GetRSockaddrKeyOfMsg(index)

	sess := r.sessions[key]
	if sess == nil {
		sess = r.createSession(key)
	} else {
		r.sessionWheel.Touch(sess, now)
	}

	if r.logger.Logger.IsLevelEnabled(logrus.DebugLevel) {
		r.logger.Debugf("upload packet: data=%v", buf)
	}

	upstreamName := r.upstreamMgr.routeUpstream(buf)
	if upstreamName == "" {
		r.logger.Info("can not route upstream")
		return
	}

	for try := 0; try < r.config.Server.MaxTryTimes; try++ {
		peer, ok := r.upstreamMgr.selectPeer(upstreamName, buf)
		if !ok {
			r.logger.Infof("upstream %s does not exist", upstreamName)
			return
		}
		if peer == nil {
			r.logger.Error("select peer failed")
			continue
		}
		fd, err := r.getOrCreateSessionFD(sess, peer.GetFamily())
		if err != nil {
			r.logger.Errorf("create session socket failed: %v", err)
			return
		}
		err = peer.Send(fd, buf)
		if err == nil {
			return
		}
		r.logger.Warnf("upload to peer %s failed: %v", peer.GetAddr(), err)
		if err != unix.EAGAIN && err != unix.EWOULDBLOCK {
			r.upstreamMgr.markPeerDead(upstreamName, peer)
		}
	}
	r.logger.Error("upload packet failed, drop it")
}

func (r *Reactor) createSession(key sd_socket.SockaddrKey) *sd_session.Session {
	sess := sd_session.NewSession(key)
	r.sessions[key] = sess
	r.sessionWheel.Add(sess, r.nowSec())
	return sess
}

func (r *Reactor) getOrCreateSessionFD(sess *sd_session.Session, family int) (int, error) {
	if fd, ok := sess.GetFDByFamily(family); ok {
		return fd, nil
	}

	fd, err := sd_socket.UDPSocket(family, true, false, false)
	if err != nil {
		return -1, err
	}
	if err = r.selector.Add(fd, sd_socket.SelectorEventRead); err != nil {
		_ = unix.Close(fd)
		return -1, err
	}

	sess.SetFD(family, fd)
	r.sessionsByFD[fd] = sess
	return fd, nil
}

func (r *Reactor) handleDownload(fd int, sess *sd_session.Session) bool {
	for batch := 0; batch < r.recvBatchLimit; batch++ {
		nPkt, errno := sd_socket.RecvMMsg(fd, r.rxMC)
		if nPkt < 1 || errno != 0 {
			if errno != unix.EAGAIN && errno != unix.EWOULDBLOCK {
				r.logger.Errorf("receive download packets failed: %s", os.NewSyscallError("recvmmsg", errno))
				r.removeSession(sess)
			}
			return false
		}

		r.sessionWheel.Touch(sess, r.nowSec())
		key := sess.GetKey()
		for i := 0; i < nPkt; i++ {
			nr := r.rxMC.GetLengthOfMsg(i)
			buf := r.rxMC.GetBufOfMsg(i)[:nr]
			r.txMC.Prepare(i, buf, key)
		}
		if err := r.sendDownloadBatch(nPkt); err != nil {
			r.logger.Errorf("send packets to downstream failed: %v", err)
		}
	}
	return true
}

func (r *Reactor) sendDownloadBatch(packetCount int) error {
	sent := 0
	for sent < packetCount {
		n, err := sd_socket.SendMMsg(r.listenFD, r.txMC, sent, packetCount)
		sent += n
		if err != nil {
			var firstErr error
			for i := sent; i < packetCount; i++ {
				if sendErr := sd_socket.SendMsg(r.listenFD, r.txMC, i); sendErr != nil && firstErr == nil {
					firstErr = sendErr
				}
			}
			return firstErr
		}
		if n < 1 {
			return fmt.Errorf("sendmmsg sent no packets")
		}
	}
	return nil
}

func (r *Reactor) expireSessions(now int64) {
	r.sessionWheel.Advance(now, r.closeSession)
}

func (r *Reactor) removeSession(sess *sd_session.Session) {
	r.sessionWheel.Remove(sess)
	r.closeSession(sess)
}

func (r *Reactor) closeSession(sess *sd_session.Session) {
	delete(r.sessions, sess.GetKey())
	for _, fd := range sess.GetFDs() {
		delete(r.sessionsByFD, fd)
		if err := r.selector.Del(fd); err != nil {
			r.logger.Errorf("delete session fd from selector failed: %v", err)
		}
		if err := unix.Close(fd); err != nil {
			r.logger.Errorf("close session fd failed: %v", err)
		}
	}
}

func (r *Reactor) close() {
	for _, sess := range r.sessions {
		r.removeSession(sess)
	}
	if err := r.selector.Del(r.listenFD); err != nil {
		r.logger.Errorf("delete listen fd from selector failed: %v", err)
	}
	if err := unix.Close(r.listenFD); err != nil {
		r.logger.Errorf("close listen fd failed: %v", err)
	}
	if err := r.selector.Close(); err != nil {
		r.logger.Errorf("close selector failed: %v", err)
	}
}
