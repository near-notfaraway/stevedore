package sd_session

import (
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"golang.org/x/sys/unix"
	"sync/atomic"
	"time"
)

type Session struct {
	name       string                // string converted from downstream address
	sa         unix.Sockaddr         // downstream sockaddr
	lastActive int64                 // last active timestamp base on second
	fd         int                   // fd used to upload packet
	upstream   *sd_upstream.Upstream // destination upstream
	peer       *sd_upstream.Peer     // destination peer of upstream
}

func NewSession(name string, sa unix.Sockaddr) *Session {
	return &Session{
		name:       name,
		sa:         sa,
		lastActive: time.Now().Unix(),
	}
}

func (s *Session) Init() {

}

func (s *Session) GetUpstream() *sd_upstream.Upstream {
	return s.upstream
}

func (s *Session) SetUpstream(upstream *sd_upstream.Upstream) {
	s.upstream = upstream
}

func (s *Session) GetPeer() *sd_upstream.Peer {
	return s.peer
}

func (s *Session) SetPeer(peer *sd_upstream.Peer) {
	s.peer = peer
}

func (s *Session) UpdateActive() {
	atomic.StoreInt64(&s.lastActive, time.Now().Unix())
}

func (s *Session) LastActive() int64 {
	return atomic.LoadInt64(&s.lastActive)
}

func (s *Session) GetFD() int {
	return s.fd
}
