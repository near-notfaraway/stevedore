package sd_upstream

import (
	"fmt"
	"github.com/near-notfaraway/stevedore/sd_config"
)

const DefaultUpstreamName = "default"

type Upstream struct {
	name  string
	peers []*Peer
}

func NewUpstream(config *sd_config.UpstreamConfig) *Upstream {
	peers := make([]*Peer, 0, len(config.Peers))
	uniqueMap := make(map[string]struct{})

	for id, peerConfig := range config.Peers {
		// check upstream addr duplication
		peerAddr := fmt.Sprintf("%s:%d", peerConfig.IP, peerConfig.Port)
		if _, dup := uniqueMap[peerAddr]; dup {
			panic(fmt.Errorf("duplicated peer %s in upstream %s", peerAddr, config.Name))
		}
		uniqueMap[peerAddr] = struct{}{}

		// create upstream
		peer := NewPeer(id, peerAddr, peerConfig)
		peers = append(peers, peer)
	}

	return &Upstream{
		name:  config.Name,
		peers: peers,
	}
}

func (u *Upstream) SelectPeer() *Peer {

}
