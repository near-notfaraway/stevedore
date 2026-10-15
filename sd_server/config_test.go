package sd_server

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/stretchr/testify/require"
)

func TestValidateConfigAcceptsValidConfigWithoutOpeningSockets(t *testing.T) {
	patches := gomonkey.ApplyFunc(
		sd_socket.UDPSocket,
		func(int, bool, bool, bool) (int, error) {
			t.Fatal("configuration validation must not open sockets")
			return -1, nil
		},
	)
	defer patches.Reset()

	require.NoError(t, ValidateConfig(validTestConfig()))
}

func TestValidateConfigRejectsInvalidUpstream(t *testing.T) {
	config := validTestConfig()
	config.Upload.Upstreams[0].Type = "invalid"

	err := ValidateConfig(config)

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid upstream type")
}

func validTestConfig() *sd_config.Config {
	return &sd_config.Config{
		Common: &sd_config.CommonConfig{
			WorkingDir: "/tmp",
			PidPath:    "/tmp/stevedore.pid",
		},
		PProf: &sd_config.PProfConfig{},
		Log: &sd_config.LogConfig{
			Path:             "/tmp/stevedore.log",
			Level:            "info",
			MaxAgeHour:       24,
			RotationTimeHour: 24,
		},
		Server: &sd_config.ServerConfig{
			ListenAddr:          "127.0.0.1:2614",
			ListenParallel:      1,
			EventSize:           128,
			BatchSize:           32,
			BufSize:             4096,
			MaxRecvBatchesPerFD: 8,
			MaxTryTimes:         3,
		},
		Upload: &sd_config.UploadConfig{
			DefaultUpstream: "default",
			Upstreams: []*sd_config.UpstreamConfig{{
				Name: "default",
				Type: sd_upstream.UpstreamTypeRR,
				Peers: []*sd_config.PeerConfig{{
					IP:     "127.0.0.1",
					Port:   2345,
					Weight: 1,
				}},
				HealthChecker: &sd_config.HealthCheckerConfig{
					HeartbeatIntervalSec: 5,
					HeartbeatTimeoutSec:  1,
					SuccessTimes:         1,
					FailedTimes:          1,
				},
			}},
		},
		Session: &sd_config.SessionConfig{
			RecycleIntervalSec: 10,
			TimeoutSec:         30,
		},
	}
}
