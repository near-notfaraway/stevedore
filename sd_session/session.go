package sd_session

import (
	"context"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"sync"
	"sync/atomic"
	"time"
)

type Session struct {
	ctx        context.Context    // control download worker close
	cancel     context.CancelFunc // ctx cancel function
	name       string             // string converted from downstream address
	sa         unix.Sockaddr      // downstream sockaddr
	lastActive int64              // last active timestamp base on second
	fd         int                // fd used to upload packet
	ch         chan struct{}      // fd used to recv download event
}

func NewSession(name string, sa unix.Sockaddr) *Session {
	ctx, cancel := context.WithCancel(context.Background())

	return &Session{
		ctx:        ctx,
		cancel:     cancel,
		name:       name,
		sa:         sa,
		lastActive: time.Now().Unix(),
	}
}

func (s *Session) UpdateActive() {
	atomic.StoreInt64(&s.lastActive, time.Now().Unix())
}

func (s *Session) LastActive() int64 {
	return atomic.LoadInt64(&s.lastActive)
}

func (s *Session) GetCtx() context.Context {
	return s.ctx
}

func (s *Session) GetName() string {
	return s.name
}

func (s *Session) GetSockaddr() unix.Sockaddr {
	return s.sa
}

func (s *Session) GetFD() int {
	return s.fd
}

func (s *Session) GetCh() chan struct{} {
	return s.ch
}

func (s *Session) Close(selector sd_socket.Selector, evChanPool sync.Pool) {
	if err := selector.Del(s.fd); err != nil {
		logrus.Errorf("delete fd from selector failed: %v", err)
	}
	s.cancel()
	evChanPool.Put(s.ch)
	if err := unix.Close(s.fd); err != nil {
		logrus.Errorf("close fd failed: %v", err)
	}
}
