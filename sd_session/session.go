package sd_session

import (
	"golang.org/x/sys/unix"
	"sync/atomic"
	"time"
)

type Session struct {
	name       string        // string converted from downstream address
	sa         unix.Sockaddr // downstream sockaddr
	lastActive int64         // last active timestamp base on second
	fd         int           // fd used to upload packet
	ch         chan struct{} // fd used to recv download event
}

func NewSession(name string, sa unix.Sockaddr) *Session {
	return &Session{
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
