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
				nPkt, err := sd_socket.RecvMMsg(sess.GetFD(), mc)
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

					// 发送数据回 client
					err = unix.Sendto(s.workers[0].fd, buf[:nr], 0, sess.GetSockaddr())
					if err != nil {
						logrus.Error("write to udp fail: %v", err)
					}
				}
			}
		}
	}
}