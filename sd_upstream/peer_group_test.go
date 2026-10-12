package sd_upstream

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPeerGroupRefreshIncludesHealthyBackup(t *testing.T) {
	primary := &Peer{addr: "127.0.0.1:10001", state: int32(PeerAlive)}
	backup := &Peer{addr: "127.0.0.1:10002", state: int32(PeerAlive)}
	group := &PeerGroup{
		peers:  []*Peer{primary, backup},
		backup: backup,
	}

	group.refresh()

	require.Equal(t, []*Peer{primary, backup}, group.Snapshot().Peers)
}

func TestPeerGroupRefreshForcesBackupWhenAllPeersAreDead(t *testing.T) {
	primary := &Peer{addr: "127.0.0.1:10001", state: int32(PeerDead)}
	backup := &Peer{addr: "127.0.0.1:10002", state: int32(PeerDead)}
	group := &PeerGroup{
		peers:  []*Peer{primary, backup},
		backup: backup,
	}

	group.refresh()

	require.Equal(t, []*Peer{backup}, group.Snapshot().Peers)
	require.Equal(t, PeerTemp, PeerState(backup.state))
}

func TestPeerGroupRefreshPublishesEmptySnapshotWithoutBackup(t *testing.T) {
	group := &PeerGroup{
		peers: []*Peer{
			{addr: "127.0.0.1:10001", state: int32(PeerDead)},
		},
	}

	group.refresh()

	require.Empty(t, group.Snapshot().Peers)
}
