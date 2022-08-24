package sd_server

import (
	"code.byted.org/gopkg/logs"
	"context"
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_socket"
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
						fmt.Errorf("%w", os.NewSyscallError("recvmmsg", err))
					}
					break
				}

				// process packets one by one
				for i := 0; i < nPkt; i++ {
					nr := mc.GetLengthOfMsg(i)
					buf := mc.GetBufOfMsg(i)
					rName := mc.GetRNamesOfMsg(i)
					rSockaddr := mc.GetRSockaddrOfMsg(i)

					sess := s.sessionMgr.GetSession(rName)
					if sess == nil {
						sess, got := s.sessionMgr.GetOrCreateSession(rName, rSockaddr)
						if !got {
							sess.Init()
						}
					}

					for try := 0; try < s.config.Server.MaxTryTimes; try++ {
						// 获取 upstream
						_upstream := sess.GetUpstream()
						if _upstream == nil {
							if _upstream, err = s.upstreamMgr.Route(hdr.Cid); err != nil {
								logs.Errorf("route to upstream failed: %v", err)
								break
							}
							sess.SetUpstream(_upstream)
						}

						// 发送数据到 upstream
						err := _upstream.Send(sess.upstreamFdIdx, buf[:nr])
						if err != nil {
							if err == UpstreamDeadErr {
								continue
							}
							logs.Errorf("send to upstream %s failed: %v", _upstream.addr.String(), err)
						}

						try ++
						continue
					}
				}
			}
		}
	}
