package sd_server

import (
	"context"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"os"
)

func (s *Server) uploadWorker(ctx context.Context, worker *UploadWorker) {
	// init logger for upload worker
	logger := logrus.WithField("work_id", worker.id)
	logger.Debug("init upload worker")
	mc := s.mcPool.GetMMsgContainerFromPool()
	defer s.mcPool.PutMMsgContainerToPool(mc)

	logger.Debug("wait for read event until ctx canceled")
	for {
		select {
		case <-ctx.Done():
			return

		case <-worker.ch:
			logger.Debug("a read event came in, continue batch recv packets")
			for {
				logger.Debug("do batch recv packets")
				nPkt, errno := sd_socket.RecvMMsg(worker.fd, mc)
				if nPkt < 1 || errno != 0 {
					if errno == unix.EAGAIN || errno == unix.EWOULDBLOCK {
						logger.Debug("no packets to recv, should wait for recv event again")
					} else {
						logger.Errorf("recv packets failed: %s", os.NewSyscallError("recvmmsg", errno).Error())
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

					// get session or create session
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

							s.fdReadHandlers.Store(_sess.GetFD(), func() { _sess.GetCh() <- struct{}{} })
							if err = s.selector.Add(_sess.GetFD(), sd_socket.SelectorEventRead); err != nil {
								logger.Errorf("add selector for session failed: %w", err)
								s.fdReadHandlers.Delete(_sess.GetFD())
								continue
							}
							go s.downloadWorker(_sess.GetCtx(), _sess)

						} else {
							logger.Debugf("session %p for packet is existed", _sess)
						}
						sess = _sess

					} else {
						logger.Debugf("session %p for packet is existed", sess)
					}

					// get upstream
					upstream := s.upstreamMgr.RouteUpstream(buf[:nr])
					if upstream == nil {
						logger.Info("can not route upstream")
						continue
					}

					logger.Debugf("try to get peer and send data to it")
					succeed := false
					for try := 0; try < s.config.Server.MaxTryTimes; try++ {
						peer := upstream.SelectPeer(buf[:nr])
						if peer == nil {
							logrus.Errorf("select peer failed")
							continue
						}

						err := peer.Send(sess.GetFD(), buf[:nr])
						if err != nil {
							logrus.Warnf("upload to peer %s succeed", peer.GetAddr())
							if err != unix.EAGAIN && err != unix.EWOULDBLOCK {
								peer.SetState(sd_upstream.PeerDead)
							}
							continue
						}

						logrus.Debugf("upload to peer %s succeed", peer.GetAddr())
						succeed = true
						break
					}

					// upload failed
					if !succeed {
						logrus.Error("upload packet failed, drop it")
					}
				}
			}
		}
	}
}
