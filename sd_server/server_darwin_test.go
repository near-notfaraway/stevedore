//go:build darwin
// +build darwin

package sd_server

import (
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/stretchr/testify/require"
)

func TestNewServerDarwinUsesSingleReactor(t *testing.T) {
	config := &sd_config.Config{
		Server: &sd_config.ServerConfig{
			ListenAddr:     "127.0.0.1:0",
			ListenParallel: 4,
			EventSize:      8,
			BatchSize:      32,
			BufSize:        4096,
		},
		Upload: &sd_config.UploadConfig{},
		Session: &sd_config.SessionConfig{
			RecycleIntervalSec: 1,
			TimeoutSec:         1,
		},
	}

	server := NewServer(config)
	t.Cleanup(func() {
		for _, reactor := range server.reactors {
			reactor.close()
		}
	})

	require.Len(t, server.reactors, 1)
}
