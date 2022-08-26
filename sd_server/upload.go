package sd_server

import (
	"context"
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_selector"
	"github.com/near-notfaraway/stevedore/sd_session"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"os"
)

func (s *Server) uploadWorker(ctx context.Context, ins *WorkerIns) {
	logger := logrus.WithField("work_id", ins.id)
	logger.Debug("init worker")
	mc := s.mcPool.GetMMsgContainerFromPool()
	defer s.mcPool.PutMMsgContainerToPool(mc)

	logger.Debug("wait for read event until ctx canceled")
	for {
		select {
		case <-ctx.Done():
			return

		case <-ins.ch:
			for {
				logger.Debug("a read event came in, batch recv packets")
				nPkt, errno := sd_socket.RecvMMsg(ins.fd, mc)
				if nPkt < 1 || errno != 0 {
					if errno == unix.EAGAIN || errno == unix.EWOULDBLOCK {
						logger.Debug("no packets to recv, should wait for recv event again")
					} else {
						logger.Errorf("recv packets failed: %w", os.NewSyscallError("recvmmsg", errno))
					}
					break
				}

				logger.Debugf("recv %d packets, process packets one by one", nPkt)
				for i := 0; i < nPkt; i++ {
					logger.Debugf("processing packet %d and extract info", i)
					nr := mc.GetLengthOfMsg(i)
					buf := mc.GetBufOfMsg(i)
					rName := mc.GetRNamesOfMsg(i)
					rSockaddr := mc.GetRSockaddrOfMsg(i)
					logger.Debugf("packet info: remote addr is %v, data is %v",
						sd_socket.SockaddrToUDPAddr(rSockaddr).String(), buf[:nr])

					// get session
					sess := s.sessionMgr.GetSession(rName)
					if sess == nil {
						logger.Debugf("try create new session for packet")
						var got bool
						_sess, got, err := s.sessionMgr.GetOrCreateSession(rName, rSockaddr)
						if err != nil {
							logger.Errorf("create session failed: %w,", err)
							continue
						}

						if !got {
							logger.Debugf("init new session %p for packet", _sess)
							fs := [2]func(){func() { _sess.GetCh() <- struct{}{} }, nil}
							err = s.selector.Add(_sess.GetFD(), sd_selector.SelectorEventRead, fs)
							if err != nil {
								logger.Errorf("add selector for session failed: %w", err)
								continue
							}
							go s.downloadWorker(s.ctx, _sess)

						} else {
							logger.Debugf("session %p for packet is existed", _sess)
						}
						sess = _sess

					} else {
						logger.Debugf("session %p for packet is existed", sess)
					}

					logger.Debugf("try to get peer and send data to it")
					succeed := false
					for try := 0; try < s.config.Server.MaxTryTimes; try++ {
						peer, err := s.selectPeer(sess, buf[:nr])
						if err != nil {
							logrus.Errorf("%w: data: %v", err, buf[:nr])
							break
						}

						err = peer.Send(sess.GetFD(), buf[:nr])
						if err != nil {
							if err != unix.EAGAIN && err != unix.EWOULDBLOCK {
								peer.SetState(sd_upstream.PeerDead)
							}
							continue
						}

						succeed = true
						break
					}

					// upload failed
					if !succeed {
						logrus.Errorf("upload failed, packet data: %v", buf[:nr])
					}
					break
				}
			}
		}
	}
}

func (s *Server) selectPeer(sess *sd_session.Session, data []byte) (*sd_upstream.Peer, error) {
	// use cache peer
	peer := sess.GetPeer()
	if peer != nil {
		return peer, nil
	}

	// use cache upstream or route upstream
	upstream := sess.GetUpstream()
	if upstream == nil {
		upstream = s.upstreamMgr.RouteUpstream(data)
		if upstream == nil {
			return nil, fmt.Errorf("route upstream failed")
		}
		sess.SetUpstream(upstream)
	}

	// select peer
	peer = upstream.SelectPeer()
	if peer == nil {
		return nil, fmt.Errorf("select peer failed")
	}
	sess.SetPeer(peer)

	return peer, nil
}
