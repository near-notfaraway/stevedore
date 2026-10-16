package sd_diagnosis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
	"github.com/smartystreets/goconvey/convey"
)

func TestInitLoggerConvey(t *testing.T) {
	convey.Convey("Test InitLogger", t, func() {
		convey.Convey("writes logs to the configured path", func() {
			logPath := filepath.Join(t.TempDir(), "stevedore.log")
			err := InitLogger(&sd_config.LogConfig{
				Path:             logPath,
				Level:            "debug",
				Verbose:          false,
				MaxAgeHour:       10,
				RotationTimeHour: 1,
			})
			convey.So(err, convey.ShouldBeNil)

			logrus.Debug("test")
			file, err := os.Open(logPath)
			convey.So(err, convey.ShouldBeNil)
			fileStat, err := file.Stat()
			convey.So(err, convey.ShouldBeNil)
			convey.So(fileStat.Size(), convey.ShouldBeGreaterThan, int64(0))
			convey.So(file.Close(), convey.ShouldBeNil)
		})

		convey.Convey("resolves a relative path", func() {
			logDir, err := os.MkdirTemp(".", "relative-log-")
			convey.So(err, convey.ShouldBeNil)
			t.Cleanup(func() {
				_ = os.RemoveAll(logDir)
			})

			logPath := filepath.Join(".", filepath.Base(logDir), "stevedore.log")
			err = InitLogger(&sd_config.LogConfig{
				Path:             logPath,
				Level:            "debug",
				MaxAgeHour:       1,
				RotationTimeHour: 1,
			})
			convey.So(err, convey.ShouldBeNil)

			logrus.Debug("test relative log path")
			content, err := os.ReadFile(logPath)
			convey.So(err, convey.ShouldBeNil)
			convey.So(string(content), convey.ShouldContainSubstring, "test relative log path")
		})

		convey.Convey("rejects an invalid level", func() {
			path := filepath.Join(t.TempDir(), "stevedore.log")
			err := InitLogger(&sd_config.LogConfig{
				Path:             path,
				Level:            "invalid",
				MaxAgeHour:       1,
				RotationTimeHour: 1,
			})
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "log level parse failed")
		})
	})
}
