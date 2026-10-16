package sd_diagnosis

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/near-notfaraway/stevedore/sd_route"
	"github.com/near-notfaraway/stevedore/sd_upstream"
	"github.com/sirupsen/logrus"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
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

func TestRunUploadStatsLoggerConvey(t *testing.T) {
	convey.Convey("Test RunUploadStatsLogger", t, func() {
		convey.Convey("returns when disabled", func() {
			RunUploadStatsLogger(context.Background(), 0, func() UploadStats {
				t.Fatal("disabled stats logger must not collect stats")
				return UploadStats{}
			})
		})

		convey.Convey("logs until the context is cancelled", func() {
			hook := installUploadStatsLogHook(t)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				RunUploadStatsLogger(ctx, time.Millisecond, func() UploadStats {
					return testUploadStats()
				})
				close(done)
			}()

			select {
			case entry := <-hook.entries:
				require.Equal(t, uint64(2), entry.Data["default_routes"])
			case <-time.After(time.Second):
				t.Fatal("upload stats log was not emitted")
			}

			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("upload stats logger did not stop")
			}
		})
	})
}

func TestLogUploadStatsConvey(t *testing.T) {
	convey.Convey("Test LogUploadStats", t, func() {
		convey.Convey("emits structured route and upstream fields", func() {
			hook := installUploadStatsLogHook(t)

			LogUploadStats(testUploadStats())

			select {
			case entry := <-hook.entries:
				require.Equal(t, uint64(2), entry.Data["default_routes"])
				require.Equal(t, uint64(3), entry.Data["short_payloads"])
				require.Equal(t, uint64(4), entry.Data["drops"])
				require.Equal(t, map[string]uint64{"blocked": 1}, entry.Data["routes"])
				require.Equal(t, map[string]map[string]uint64{
					"default": {
						"selections":         5,
						"selection_failures": 6,
						"peer_failures":      7,
					},
				}, entry.Data["upstreams"])
			default:
				t.Fatal("upload stats log was not emitted")
			}
		})

		convey.Convey("handles empty stats", func() {
			convey.So(func() {
				LogUploadStats(UploadStats{})
			}, convey.ShouldNotPanic)
		})
	})
}

func installUploadStatsLogHook(t testing.TB) *uploadStatsLogHook {
	t.Helper()
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
	return hook
}

func testUploadStats() UploadStats {
	return UploadStats{
		Route: sd_route.Stats{
			Routes:        []sd_route.RouteStats{{Name: "blocked", Matches: 1}},
			DefaultRoutes: 2,
			ShortPayloads: 3,
			Drops:         4,
		},
		Upstreams: []sd_upstream.Stats{{
			Name:              "default",
			Selections:        5,
			SelectionFailures: 6,
			PeerFailures:      7,
		}},
	}
}
