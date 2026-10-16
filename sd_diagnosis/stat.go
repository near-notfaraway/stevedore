package sd_diagnosis

import (
	"context"
	"time"

	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/sirupsen/logrus"
)

type UploadStats struct {
	Route     sd_route.Stats
	Upstreams []sd_upstream.Stats
}

func RunUploadStatsLogger(ctx context.Context, interval time.Duration, collect func() UploadStats) {
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			LogUploadStats(collect())
		case <-ctx.Done():
			return
		}
	}
}

func LogUploadStats(stats UploadStats) {
	routes := make(map[string]uint64, len(stats.Route.Routes))
	for _, route := range stats.Route.Routes {
		routes[route.Name] = route.Matches
	}
	upstreams := make(map[string]map[string]uint64, len(stats.Upstreams))
	for _, upstream := range stats.Upstreams {
		upstreams[upstream.Name] = map[string]uint64{
			"selections":         upstream.Selections,
			"selection_failures": upstream.SelectionFailures,
			"peer_failures":      upstream.PeerFailures,
		}
	}
	logrus.WithFields(logrus.Fields{
		"default_routes": stats.Route.DefaultRoutes,
		"short_payloads": stats.Route.ShortPayloads,
		"drops":          stats.Route.Drops,
		"routes":         routes,
		"upstreams":      upstreams,
	}).Info("upload stats")
}
