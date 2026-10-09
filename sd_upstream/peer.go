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
	weight   int           // selected ratio
	state    int32         // atomic PeerState
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
