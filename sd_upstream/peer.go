package sd_upstream

import (
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"golang.org/x/sys/unix"
	"sync"
)

type PeerState int

const (
	_ PeerState = iota
	PeerAlive
	PeerTemp
	PeerDead
)

type Peer struct {
	mu       sync.RWMutex
	id       int
	addr     string
	sockaddr unix.Sockaddr
	weight   int
	state    PeerState
}

func NewPeer(id int, addr string, config *sd_config.PeerConfig) *Peer {
	sockaddr := sd_socket.ResolveUDPSockaddr(addr)
	if sockaddr == nil {
		panic(fmt.Errorf("resolve peer addr %s failed", addr))
	}

	return &Peer{
		id:       id,
		addr:     addr,
		sockaddr: sockaddr,
		weight:   config.Weight,
		state:    PeerAlive,
	}
}

func (p *Peer) Send(fd int, data []byte) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return unix.Sendto(fd, data, 0, p.sockaddr)
}

func (p *Peer) SetState(state PeerState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = state
}

func (p *Peer) isAlive() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state == PeerAlive
}

func (p *Peer) GetAddr() string {
	return p.addr
}
