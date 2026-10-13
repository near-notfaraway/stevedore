package sd_util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
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
