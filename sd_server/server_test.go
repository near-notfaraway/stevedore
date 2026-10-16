package sd_server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_diagnosis"
	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_socket"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestServerReloadUploadConvey(t *testing.T) {
	convey.Convey("Test Server.ReloadUpload", t, func() {
		convey.Convey("switches all reactors", func() {
			oldRegistry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
			}, false)
			oldManagers := []*uploadManager{
				mustNewUploadManager(t, oldRegistry),
				mustNewUploadManager(t, oldRegistry),
			}
			server := &Server{
				uploadRegistry: oldRegistry,
				uploadManagers: oldManagers,
			}
			serveDone := make(chan error, len(oldManagers))
			for _, manager := range oldManagers {
				selector := &commandSelector{wakeCh: make(chan struct{}, 1)}
				r := newConveyReactor(manager)
				r.selector = selector
				server.reactors = append(server.reactors, r)
				go func() {
					serveDone <- r.serve(context.Background())
				}()
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := server.ReloadUpload(ctx, &sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
				Routes: []sd_config.RouteConfig{{
					Name: "reloaded",
					Conditions: []sd_config.RouteConditionConfig{{
						KeyBytes: "0:1",
						Operator: "==",
						Value:    "0x01",
					}},
					Upstream: sd_route.DropUpstream,
				}},
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				server.uploadRegistry.close()
			})

			require.Len(t, server.uploadManagers, 2)
			for i, r := range server.reactors {
				require.Same(t, server.uploadManagers[i], r.uploadMgr)
				require.NotSame(t, oldManagers[i], r.uploadMgr)
				require.Equal(t, "reloaded", r.uploadMgr.stats().Route.Routes[0].Name)
				require.NoError(t, <-serveDone)
			}
		})
		convey.Convey("rolls back a partial switch", func() {
			oldRegistry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
			}, false)
			oldManagers := []*uploadManager{
				mustNewUploadManager(t, oldRegistry),
				mustNewUploadManager(t, oldRegistry),
			}
			server := &Server{
				uploadRegistry: oldRegistry,
				uploadManagers: oldManagers,
			}
			serveCtx, stopServing := context.WithCancel(context.Background())
			serveDone := make(chan error, len(oldManagers))
			selectors := []*loopCommandSelector{
				{wakeCh: make(chan struct{}, 2)},
				{wakeCh: make(chan struct{}, 2), wakeErr: errors.New("wake failed")},
			}
			for i, manager := range oldManagers {
				r := newConveyReactor(manager)
				r.selector = selectors[i]
				server.reactors = append(server.reactors, r)
				go func() {
					serveDone <- r.serve(serveCtx)
				}()
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := server.ReloadUpload(ctx, &sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
			})
			require.Error(t, err)

			require.Same(t, oldRegistry, server.uploadRegistry)
			require.Equal(t, oldManagers, server.uploadManagers)
			for i, r := range server.reactors {
				require.Same(t, oldManagers[i], r.uploadMgr)
			}

			stopServing()
			for range server.reactors {
				require.NoError(t, <-serveDone)
			}
		})
		convey.Convey("retains the registry when rollback fails", func() {
			oldRegistry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
			}, false)
			oldManagers := []*uploadManager{
				mustNewUploadManager(t, oldRegistry),
				mustNewUploadManager(t, oldRegistry),
			}
			server := &Server{
				uploadRegistry: oldRegistry,
				uploadManagers: oldManagers,
			}
			serveCtx, stopServing := context.WithCancel(context.Background())
			serveDone := make(chan error, len(oldManagers))
			selectors := []*loopCommandSelector{
				{wakeCh: make(chan struct{}, 2), wakeErr: errors.New("rollback wake failed"), failAt: 2},
				{wakeCh: make(chan struct{}, 2), wakeErr: errors.New("wake failed")},
			}
			for i, manager := range oldManagers {
				r := newConveyReactor(manager)
				r.selector = selectors[i]
				server.reactors = append(server.reactors, r)
				go func() {
					serveDone <- r.serve(serveCtx)
				}()
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := server.ReloadUpload(ctx, &sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), "rollback failed")
			require.Len(t, server.retainedUploadRegistries, 1)

			stopServing()
			for range server.reactors {
				require.NoError(t, <-serveDone)
			}
			server.closeUploadRegistries()
		})
	})
}

func TestNewServerConvey(t *testing.T) {
	convey.Convey("Test NewServer", t, func() {
		convey.Convey("builds configured reactors", func() {
			patches := gomonkey.NewPatches()
			patches.ApplyFuncReturn(sd_socket.ResolveUDPSockaddr, &unix.SockaddrInet4{})
			patches.ApplyFunc(
				newReactor,
				func(_ int, _ *sd_config.Config, _ unix.Sockaddr, uploadMgr *uploadManager) (*reactor, error) {
					return newConveyReactor(uploadMgr), nil
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
		convey.Convey("uses one reactor without reuse-port sharding", func() {
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
		})
	})
}

func TestServerUploadStatsConvey(t *testing.T) {
	convey.Convey("Test Server.uploadStats", t, func() {
		convey.Convey("aggregates reactors", func() {
			manager := newTestUploadManager(nil, mustNewRouter(t, "", nil, true))
			manager.routeUpstream(nil)
			server := &Server{uploadManagers: []*uploadManager{manager, manager}}
			convey.So(server.uploadStats().Route.DefaultRoutes, convey.ShouldEqual, 2)
		})
		convey.Convey("returns empty stats without reactors", func() {
			require.Equal(t, sd_diagnosis.UploadStats{}, (&Server{}).uploadStats())
		})
		convey.Convey("aggregates upstream stats across managers", func() {
			registry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
				DefaultUpstream: "default",
				Upstreams: []*sd_config.UpstreamConfig{
					{
						Name: "default",
						Type: sd_upstream.UpstreamTypeRR,
						HealthChecker: &sd_config.HealthCheckerConfig{
							HeartbeatIntervalSec: 3600,
							SuccessTimes:         1,
							FailedTimes:          1,
						},
					},
				},
			}, true)

			firstManager := mustNewUploadManager(t, registry)
			secondManager := mustNewUploadManager(t, registry)
			firstName := firstManager.routeUpstream(nil)
			firstPeer, ok := firstManager.selectPeer(firstName, nil)
			require.True(t, ok)
			secondName := secondManager.routeUpstream(nil)
			secondPeer, ok := secondManager.selectPeer(secondName, nil)
			require.True(t, ok)
			require.Nil(t, firstPeer)
			require.Nil(t, secondPeer)

			managerStats := []sd_upstream.Stats{
				{
					Name:              "default",
					SelectionFailures: 1,
				},
			}
			require.Equal(t, managerStats, firstManager.stats().Upstreams)
			require.Equal(t, managerStats, secondManager.stats().Upstreams)
			server := &Server{
				uploadRegistry: registry,
				uploadManagers: []*uploadManager{firstManager, secondManager},
			}
			require.Equal(t, []sd_upstream.Stats{
				{
					Name:              "default",
					SelectionFailures: 2,
				},
			}, server.uploadStats().Upstreams)
		})
		convey.Convey("aggregates route stats across managers", func() {
			registry := mustNewUploadRegistry(t, &sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
			}, true)
			first := mustNewUploadManager(t, registry)
			second := mustNewUploadManager(t, registry)

			first.routeUpstream(nil)
			second.routeUpstream(nil)

			require.Equal(t, uint64(1), first.stats().Route.DefaultRoutes)
			require.Equal(t, uint64(1), first.stats().Route.Drops)
			require.Equal(t, uint64(1), second.stats().Route.DefaultRoutes)
			require.Equal(t, uint64(1), second.stats().Route.Drops)
			server := &Server{
				uploadRegistry: registry,
				uploadManagers: []*uploadManager{first, second},
			}
			require.Equal(t, uint64(2), server.uploadStats().Route.DefaultRoutes)
			require.Equal(t, uint64(2), server.uploadStats().Route.Drops)
		})
	})
}

func TestServerListenAndServeConvey(t *testing.T) {
	convey.Convey("Test Server.ListenAndServe", t, func() {
		convey.Convey("waits for reactor completion", func() {
			server := &Server{reactors: []*reactor{newConveyReactor(nil)}}
			patches := gomonkey.ApplyFuncReturn(unix.Close, nil)
			defer patches.Reset()
			convey.So(server.ListenAndServe(context.Background()), convey.ShouldBeNil)
		})
		convey.Convey("stops on context cancellation", func() {
			ctx, cancel := context.WithCancel(context.Background())
			selector := &loopCommandSelector{wakeCh: make(chan struct{}, 1)}
			r := newConveyReactor(nil)
			r.selector = selector
			server := &Server{reactors: []*reactor{r}}
			patches := gomonkey.ApplyFuncReturn(unix.Close, nil)
			defer patches.Reset()
			done := make(chan error, 1)
			go func() {
				done <- server.ListenAndServe(ctx)
			}()

			cancel()

			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for server shutdown")
			}
		})
	})
}

func TestAddUploadStatsConvey(t *testing.T) {
	convey.Convey("Test addUploadStats", t, func() {
		convey.Convey("sums counters", func() {
			total := sd_diagnosis.UploadStats{Route: sd_route.Stats{DefaultRoutes: 1}}
			addUploadStats(&total, sd_diagnosis.UploadStats{Route: sd_route.Stats{DefaultRoutes: 2}})
			convey.So(total.Route.DefaultRoutes, convey.ShouldEqual, 3)
		})
		convey.Convey("aggregates every counter", func() {
			total := sd_diagnosis.UploadStats{
				Route: sd_route.Stats{
					Routes:        []sd_route.RouteStats{{ID: 0, Name: "route", Matches: 1}},
					DefaultRoutes: 2,
					ShortPayloads: 3,
					Drops:         4,
				},
				Upstreams: []sd_upstream.Stats{{
					Name:              "upstream",
					Selections:        5,
					SelectionFailures: 6,
					PeerFailures:      7,
				}},
			}
			current := sd_diagnosis.UploadStats{
				Route: sd_route.Stats{
					Routes:        []sd_route.RouteStats{{ID: 0, Name: "route", Matches: 10}},
					DefaultRoutes: 20,
					ShortPayloads: 30,
					Drops:         40,
				},
				Upstreams: []sd_upstream.Stats{{
					Name:              "upstream",
					Selections:        50,
					SelectionFailures: 60,
					PeerFailures:      70,
				}},
			}

			addUploadStats(&total, current)

			require.Equal(t, sd_diagnosis.UploadStats{
				Route: sd_route.Stats{
					Routes:        []sd_route.RouteStats{{ID: 0, Name: "route", Matches: 11}},
					DefaultRoutes: 22,
					ShortPayloads: 33,
					Drops:         44,
				},
				Upstreams: []sd_upstream.Stats{{
					Name:              "upstream",
					Selections:        55,
					SelectionFailures: 66,
					PeerFailures:      77,
				}},
			}, total)
		})
	})
}

func TestValidateRuntimeConfigConvey(t *testing.T) {
	convey.Convey("Test ValidateRuntimeConfig", t, func() {
		convey.Convey("accepts valid config without opening sockets", func() {
			patches := gomonkey.ApplyFunc(
				sd_socket.UDPSocket,
				func(int, bool, bool, bool) (int, error) {
					t.Fatal("configuration validation must not open sockets")
					return -1, nil
				},
			)
			defer patches.Reset()

			convey.So(ValidateRuntimeConfig(validRuntimeConfig()), convey.ShouldBeNil)
		})

		convey.Convey("rejects an invalid listen address", func() {
			config := validRuntimeConfig()
			config.Server.ListenAddr = "invalid"

			err := ValidateRuntimeConfig(config)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "server listen address")
		})

		convey.Convey("rejects an invalid upstream", func() {
			config := validRuntimeConfig()
			config.Upload.Upstreams[0].Type = "invalid"

			err := ValidateRuntimeConfig(config)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "invalid upstream type")
		})
	})
}

func validRuntimeConfig() *sd_config.Config {
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
