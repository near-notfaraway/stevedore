package sd_server

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

type routeStatsLogHook struct {
	entries chan *logrus.Entry
}

func (h *routeStatsLogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *routeStatsLogHook) Fire(entry *logrus.Entry) error {
	if entry.Message == "route stats" {
		select {
		case h.entries <- entry:
		default:
		}
	}
	return nil
}

func TestServerExposesRouteStats(t *testing.T) {
	registry := sd_upstream.NewRegistry(&sd_config.UploadConfig{
		DefaultUpstream: sd_route.DropUpstream,
	})
	registry.EnableRouteStats()
	registry.NewManager().RouteUpstream(nil)
	server := &Server{upstreamRegistry: registry}

	provider, ok := any(server).(interface {
		RouteStats() sd_route.Stats
	})
	require.True(t, ok)

	stats := provider.RouteStats()
	require.Equal(t, uint64(1), stats.DefaultMatches)
	require.Equal(t, uint64(1), stats.Drops)
}

func TestServerRouteStatsCollectionFollowsLoggingInterval(t *testing.T) {
	for _, test := range []struct {
		name           string
		interval       time.Duration
		defaultMatches uint64
	}{
		{name: "disabled", interval: 0, defaultMatches: 0},
		{name: "enabled", interval: time.Second, defaultMatches: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := sd_upstream.NewRegistry(&sd_config.UploadConfig{
				DefaultUpstream: sd_route.DropUpstream,
			})
			server := &Server{upstreamRegistry: registry}
			configurer, ok := any(server).(interface {
				configureRouteStats(time.Duration)
			})
			require.True(t, ok)

			configurer.configureRouteStats(test.interval)
			registry.NewManager().RouteUpstream(nil)

			require.Equal(t, test.defaultMatches, server.RouteStats().DefaultMatches)
		})
	}
}

func TestServerLogsRouteStatsUntilContextCancelled(t *testing.T) {
	registry := sd_upstream.NewRegistry(&sd_config.UploadConfig{
		DefaultUpstream: sd_route.DropUpstream,
		Routes: []sd_config.RouteConfig{
			{
				Name: "blocked",
				Conditions: []sd_config.RouteConditionConfig{
					{KeyBytes: "0:1", Operator: "==", Value: "0x01"},
				},
				Upstream: sd_route.DropUpstream,
			},
		},
	})
	server := &Server{upstreamRegistry: registry}
	runner, ok := any(server).(interface {
		configureRouteStats(time.Duration)
		runRouteStatsLogger(context.Context)
	})
	require.True(t, ok)
	runner.configureRouteStats(time.Millisecond)
	registry.NewManager().RouteUpstream([]byte{0x01})

	logger := logrus.StandardLogger()
	oldHooks := logger.ReplaceHooks(make(logrus.LevelHooks))
	oldOutput := logger.Out
	oldLevel := logger.Level
	t.Cleanup(func() {
		logger.ReplaceHooks(oldHooks)
		logger.SetOutput(oldOutput)
		logger.SetLevel(oldLevel)
	})
	hook := &routeStatsLogHook{entries: make(chan *logrus.Entry, 1)}
	logger.AddHook(hook)
	logger.SetOutput(io.Discard)
	logger.SetLevel(logrus.InfoLevel)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runner.runRouteStatsLogger(ctx)
		close(done)
	}()

	select {
	case entry := <-hook.entries:
		require.Equal(t, uint64(0), entry.Data["default_matches"])
		require.Equal(t, uint64(0), entry.Data["short_payloads"])
		require.Equal(t, uint64(1), entry.Data["drops"])
		require.Equal(t, map[string]uint64{"blocked": 1}, entry.Data["routes"])
	case <-time.After(time.Second):
		t.Fatal("route stats log was not emitted")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("route stats logger did not stop")
	}
}
