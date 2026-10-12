package sd_upstream

import (
	"fmt"
	"sync/atomic"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"golang.org/x/sys/unix"
)

type PeerState int32

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
	id       int           // unique id
	addr     string        // humane addr
	sockaddr unix.Sockaddr // sockaddr for send packet
	family   int           // sockaddr family
	weight   int           // selected ratio
	state    int32         // atomic PeerState
}

func NewPeer(id int, addr string, config *sd_config.PeerConfig) *Peer {
	sockaddr := sd_socket.ResolveUDPSockaddr(addr)
	if sockaddr == nil {
		panic(fmt.Errorf("resolve peer addr %s failed", addr))
	}

	family := unix.AF_INET
	if _, ok := sockaddr.(*unix.SockaddrInet6); ok {
		family = unix.AF_INET6
	}

	return &Peer{
		id:       id,
		addr:     addr,
		sockaddr: sockaddr,
		family:   family,
		weight:   config.Weight,
		state:    int32(PeerAlive),
	}
}

func (p *Peer) Send(fd int, data []byte) error {
	return sd_socket.SendTo(fd, data, 0, p.sockaddr)
}

func (p *Peer) SetState(state PeerState) {
	atomic.StoreInt32(&p.state, int32(state))
}

func (p *Peer) isAlive() bool {
	return PeerState(atomic.LoadInt32(&p.state)) == PeerAlive
}

func (p *Peer) GetAddr() string {
	return p.addr
}

func (p *Peer) GetFamily() int {
	return p.family
}
