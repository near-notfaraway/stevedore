package sd_server

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/sirupsen/logrus"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

type uploadStatsLogHook struct {
	entries chan *logrus.Entry
}

func (h *uploadStatsLogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *uploadStatsLogHook) Fire(entry *logrus.Entry) error {
	if entry.Message == "upload stats" {
		select {
		case h.entries <- entry:
		default:
		}
	}
	return nil
}

func TestServerUploadStatsWithoutReactors(t *testing.T) {
	require.Equal(t, uploadStats{}, (&Server{}).uploadStats())
}

func TestNewServerUsesSingleReactorWithoutReusePortSharding(t *testing.T) {
	if sd_socket.ReusePortShardingSupported() {
		t.Skip("platform supports SO_REUSEPORT sharding")
	}
	config := &sd_config.Config{
		Server: &sd_config.ServerConfig{
			ListenAddr:     "127.0.0.1:0",
			ListenParallel: 4,
			EventSize:      8,
			BatchSize:      32,
			BufSize:        4096,
		},
		Upload: &sd_config.UploadConfig{
			DefaultUpstream: sd_route.DropUpstream,
		},
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

func TestAddUploadStatsAggregatesAllCounters(t *testing.T) {
	total := uploadStats{
		route: sd_route.Stats{
			Routes:        []sd_route.RouteStats{{ID: 0, Name: "route", Matches: 1}},
			DefaultRoutes: 2,
			ShortPayloads: 3,
			Drops:         4,
		},
		upstreams: []sd_upstream.Stats{{
			Name:              "upstream",
			Selections:        5,
			SelectionFailures: 6,
			PeerFailures:      7,
		}},
	}
	current := uploadStats{
		route: sd_route.Stats{
			Routes:        []sd_route.RouteStats{{ID: 0, Name: "route", Matches: 10}},
			DefaultRoutes: 20,
			ShortPayloads: 30,
			Drops:         40,
		},
		upstreams: []sd_upstream.Stats{{
			Name:              "upstream",
			Selections:        50,
			SelectionFailures: 60,
			PeerFailures:      70,
		}},
	}

	addUploadStats(&total, current)

	require.Equal(t, uploadStats{
		route: sd_route.Stats{
			Routes:        []sd_route.RouteStats{{ID: 0, Name: "route", Matches: 11}},
			DefaultRoutes: 22,
			ShortPayloads: 33,
			Drops:         44,
		},
		upstreams: []sd_upstream.Stats{{
			Name:              "upstream",
			Selections:        55,
			SelectionFailures: 66,
			PeerFailures:      77,
		}},
	}, total)
}

func TestServerLogsUploadStatsUntilContextCancelled(t *testing.T) {
	registry := newUploadRegistry(&sd_config.UploadConfig{
		DefaultUpstream: "default",
		Upstreams: []*sd_config.UpstreamConfig{
			{
				Name: "default",
				Type: sd_upstream.UpstreamTypeRR,
				HealthChecker: &sd_config.HealthCheckerConfig{
					HeartbeatIntervalSec: 3600,
				},
			},
		},
		Routes: []sd_config.RouteConfig{
			{
				Name: "blocked",
				Conditions: []sd_config.RouteConditionConfig{
					{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
				},
				Upstream: sd_route.DropUpstream,
			},
		},
	}, true)
	server := &Server{
		uploadRegistry:      registry,
		uploadStatsInterval: time.Millisecond,
	}
	manager := registry.newManager()
	server.reactors = []*reactor{{upstreamMgr: manager}}
	manager.routeUpstream([]byte{0x01})
	upstreamName := manager.routeUpstream([]byte{0x02})
	require.Equal(t, "default", upstreamName)
	peer, ok := manager.selectPeer(upstreamName, nil)
	require.True(t, ok)
	require.Nil(t, peer)

	logger := logrus.StandardLogger()
	oldHooks := logger.ReplaceHooks(make(logrus.LevelHooks))
	oldOutput := logger.Out
	oldLevel := logger.Level
	t.Cleanup(func() {
		logger.ReplaceHooks(oldHooks)
		logger.SetOutput(oldOutput)
		logger.SetLevel(oldLevel)
	})
	hook := &uploadStatsLogHook{entries: make(chan *logrus.Entry, 1)}
	logger.AddHook(hook)
	logger.SetOutput(io.Discard)
	logger.SetLevel(logrus.InfoLevel)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		server.runUploadStatsLogger(ctx)
		close(done)
	}()

	select {
	case entry := <-hook.entries:
		require.Equal(t, uint64(1), entry.Data["default_routes"])
		require.Equal(t, uint64(0), entry.Data["short_payloads"])
		require.Equal(t, uint64(1), entry.Data["drops"])
		require.Equal(t, map[string]uint64{"blocked": 1}, entry.Data["routes"])
		require.Equal(t, map[string]map[string]uint64{
			"default": {
				"selections":         0,
				"selection_failures": 1,
				"peer_failures":      0,
			},
		}, entry.Data["upstreams"])
	case <-time.After(time.Second):
		t.Fatal("upload stats log was not emitted")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("upload stats logger did not stop")
	}
}

func TestNewServerConvey(t *testing.T) {
	convey.Convey("Test NewServer", t, func() {
		convey.Convey("builds configured reactors", func() {
			patches := gomonkey.NewPatches()
			patches.ApplyFuncReturn(sd_socket.ResolveUDPSockaddr, &unix.SockaddrInet4{})
			patches.ApplyFunc(
				newReactor,
				func(id int, config *sd_config.Config, listenSa unix.Sockaddr, manager *uploadManager) (*reactor, error) {
					return newConveyReactor(manager), nil
				},
			)
			defer patches.Reset()
			server := NewServer(&sd_config.Config{
				Server:  &sd_config.ServerConfig{ListenAddr: "mock", ListenParallel: 1},
				Upload:  &sd_config.UploadConfig{DefaultUpstream: sd_route.DropUpstream},
				Session: &sd_config.SessionConfig{},
			})
			convey.So(server.reactors, convey.ShouldHaveLength, 1)
		})
	})
}

func TestServerRunUploadStatsLoggerConvey(t *testing.T) {
	convey.Convey("Test Server.runUploadStatsLogger", t, func() {
		convey.Convey("returns when disabled", func() {
			server := &Server{}
			server.runUploadStatsLogger(context.Background())
			convey.So(server.uploadStatsInterval, convey.ShouldEqual, 0)
		})
	})
}

func TestServerLogUploadStatsConvey(t *testing.T) {
	convey.Convey("Test Server.logUploadStats", t, func() {
		convey.Convey("handles empty stats", func() {
			convey.So(func() {
				(&Server{}).logUploadStats()
			}, convey.ShouldNotPanic)
		})
	})
}

func TestServerUploadStatsConvey(t *testing.T) {
	convey.Convey("Test Server.uploadStats", t, func() {
		convey.Convey("aggregates reactors", func() {
			manager := newTestUploadManager(nil, sd_route.NewRouter("", nil, true))
			manager.routeUpstream(nil)
			server := &Server{reactors: []*reactor{{upstreamMgr: manager}, {upstreamMgr: manager}}}
			convey.So(server.uploadStats().route.DefaultRoutes, convey.ShouldEqual, 2)
		})
	})
}

func TestServerListenAndServeConvey(t *testing.T) {
	convey.Convey("Test Server.ListenAndServe", t, func() {
		convey.Convey("waits for reactor completion", func() {
			server := &Server{reactors: []*reactor{newConveyReactor(nil)}}
			patches := gomonkey.ApplyFuncReturn(unix.Close, nil)
			defer patches.Reset()
			convey.So(server.ListenAndServe(), convey.ShouldBeNil)
		})
	})
}

func TestAddUploadStatsConvey(t *testing.T) {
	convey.Convey("Test addUploadStats", t, func() {
		convey.Convey("sums counters", func() {
			total := uploadStats{route: sd_route.Stats{DefaultRoutes: 1}}
			addUploadStats(&total, uploadStats{route: sd_route.Stats{DefaultRoutes: 2}})
			convey.So(total.route.DefaultRoutes, convey.ShouldEqual, 3)
		})
	})
}
