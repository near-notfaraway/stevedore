package sd_server

import (
	"context"
	"github.com/near-notfaraway/stevedore/sd_session"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"os"
)

func (s *Server) downloadWorker(ctx context.Context, sess *sd_session.Session) {
	logger := logrus.WithField("session_name", sess.GetName())
	mc := s.mcPool.GetMMsgContainerFromPool()
	defer s.mcPool.PutMMsgContainerToPool(mc)

	// recv selector event util ctx cancel
	for {
		select {
		case <-ctx.Done():
			return

		case <-sess.GetCh():
			for {
				// recv packets in batches
				nPkt, errno := sd_socket.RecvMMsg(sess.GetFD(), mc)
				if nPkt < 1 || errno != 0 {
					if errno == unix.EAGAIN || errno == unix.EWOULDBLOCK {
						logger.Debug("no packets to recv, should wait for recv event again")
					} else {
						logger.Errorf("recv packets failed: %w", os.NewSyscallError("recvmmsg", errno))
					}
					break
				}

				// process packets one by one
				for i := 0; i < nPkt; i++ {
					nr := mc.GetLengthOfMsg(i)
					buf := mc.GetBufOfMsg(i)

					// 发送数据回 client
					err := unix.Sendto(s.workers[0].fd, buf[:nr], 0, sess.GetSockaddr())
					if err != nil {
						logrus.Error("write to udp fail: %v", err)
					}
				}

				break
			}
		}
	}
}
