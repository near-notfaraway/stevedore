package sd_diagnosis

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/lestrrat/go-file-rotatelogs"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
)

func InitLogger(conf *sd_config.LogConfig) error {
	logPath, err := filepath.Abs(conf.Path)
	if err != nil {
		return fmt.Errorf("resolve log path failed: %w", err)
	}

	writer, err := rotatelogs.New(
		logPath+".%Y%m%d",
		rotatelogs.WithLinkName(logPath),
		rotatelogs.WithMaxAge(time.Hour*time.Duration(conf.MaxAgeHour)),
		rotatelogs.WithRotationTime(time.Hour*time.Duration(conf.RotationTimeHour)))
	if err != nil {
		return fmt.Errorf("log rotation build failed: %w", err)
	}

	level, err := logrus.ParseLevel(conf.Level)
	if err != nil {
		return fmt.Errorf("log level parse failed: %w", err)
	}

	// set logger
	logrus.SetOutput(writer)
	logrus.SetLevel(level)
	logrus.SetReportCaller(conf.Verbose)
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceQuote: true,
	})

	return nil
}
