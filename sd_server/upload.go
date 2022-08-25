package sd_server

import (
	"context"
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_session"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"os"
)

func (s *Server) uploadWorker(ctx context.Context, ins *WorkerIns) {
	mc := s.mcPool.GetMMsgContainerFromPool()
	defer s.mcPool.PutMMsgContainerToPool(mc)

	// recv selector event util ctx cancel
	for {
		select {
		case <-ctx.Done():
			return

		case <-ins.ch:
			for {
				// recv packets in batches
				nPkt, err := sd_socket.RecvMMsg(ins.fd, mc)
				if nPkt < 1 || err != nil {
					if err != unix.EAGAIN && err != unix.EWOULDBLOCK {
						logrus.Errorf("recv upload packet failed: %w", os.NewSyscallError("recvmmsg", err))
					}
					break
				}

				// process packets one by one
				for i := 0; i < nPkt; i++ {
					nr := mc.GetLengthOfMsg(i)
					buf := mc.GetBufOfMsg(i)
					rName := mc.GetRNamesOfMsg(i)
					rSockaddr := mc.GetRSockaddrOfMsg(i)

					// get session
					sess := s.sessionMgr.GetSession(rName)
					if sess == nil {
						sess, got := s.sessionMgr.GetOrCreateSession(rName, rSockaddr)
						if !got {
							sess.Init()
						}
					}

					// try to get and send peer
					for try := 0; try < s.config.Server.MaxTryTimes; try++ {
						peer, err := s.selectPeer(sess, buf[:nr])
						if err != nil {
							logrus.Errorf("select peer failed: %v", buf[:nr])
							break
						}

						err = peer.Send(sess.GetFD(), buf[:nr])
						if err != nil {
							if err != unix.EAGAIN && err != unix.EWOULDBLOCK {
								peer.SetState(sd_upstream.PeerDead)
							}
							continue
						}

						break
					}

					// upload failed
					logrus.Errorf("upload failed, packet data: %v", buf[:nr])
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
