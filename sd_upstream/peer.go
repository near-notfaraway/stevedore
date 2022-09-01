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

//------------------------------------------------------------------------------
// Peer: Used to upload packet
//------------------------------------------------------------------------------

type Peer struct {
	mu       sync.RWMutex  // lock
	id       int           // unique id
	addr     string        // humane addr
	sockaddr unix.Sockaddr // sockaddr for send packet
	weight   int           // selected ratio
	state    PeerState     // define if peer is available
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
	return sd_socket.SendTo(fd, data, 0, p.sockaddr)
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
