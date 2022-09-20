package sd_diagnosis

import (
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"testing"
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
