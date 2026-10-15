package sd_diagnosis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitLogger(t *testing.T) {
	err := InitLogger(&sd_config.LogConfig{
		Path:             "./tmp.log",
		Level:            "debug",
		Verbose:          false,
		MaxAgeHour:       10,
		RotationTimeHour: 1,
	})

	// log it and check file
	assert.Nil(t, err)
	logrus.Debug("test")
	file, err := os.Open("./tmp.log")
	assert.Nil(t, err)
	fStat, err := file.Stat()
	assert.Nil(t, err)
	assert.Greater(t, fStat.Size(), int64(0))
	err = file.Close()
	assert.Nil(t, err)

	// clean
	paths, err := filepath.Glob("./tmp.log*")
	assert.Nil(t, err)
	for _, f := range paths {
		if err := os.RemoveAll(f); err != nil {
			assert.Nil(t, err)
		}
	}
}

func TestInitLoggerResolvesRelativePath(t *testing.T) {
	logDir, err := os.MkdirTemp(".", "relative-log-")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(logDir))
	})

	logPath := filepath.Join(".", filepath.Base(logDir), "stevedore.log")
	err = InitLogger(&sd_config.LogConfig{
		Path:             logPath,
		Level:            "debug",
		MaxAgeHour:       1,
		RotationTimeHour: 1,
	})
	require.NoError(t, err)

	logrus.Debug("test relative log path")
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.Contains(t, string(content), "test relative log path")
}

func TestInitLoggerConvey(t *testing.T) {
	convey.Convey("Test InitLogger", t, func() {
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
