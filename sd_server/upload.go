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
					n := mc.GetOneMsgLength(i)
					buf := mc.GetOneMsgBuf(i)
					raddr := mc.GetOneMsgRAddr(i)
				}
			}
		}
	}
}
