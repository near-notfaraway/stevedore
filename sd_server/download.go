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

	logger.Debug("wait for read event until ctx canceled")
	for {
		select {
		case <-ctx.Done():
			return

		case <-sess.GetCh():
			logger.Debug("a read event came in, continue batch recv packets")
			for {
				logger.Debug("do batch recv packets")
				nPkt, errno := sd_socket.RecvMMsg(sess.GetFD(), mc)
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
					logger.Debugf("packet info: data is %v", buf[:nr])

					logger.Debugf("send packets to downstream")
					err := sd_socket.SendTo(s.workers[0].fd, buf[:nr], 0, sess.GetSockaddr())
					if err != nil {
						logrus.Error("write to udp fail: %v", err)
					}
				}

				break
			}
		}
	}
}
