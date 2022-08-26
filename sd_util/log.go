package sd_util

import (
	"fmt"
	"github.com/lestrrat/go-file-rotatelogs"
	"github.com/near-notfaraway/stevedore/sd_config"
	"github.com/sirupsen/logrus"
	"time"
)

func InitLogger(conf *sd_config.LogConfig) error {
	// parse conf
	writer, err := rotatelogs.New(
		conf.Path+".%Y%m%d",
		rotatelogs.WithLinkName(conf.Path),
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
		ForceQuote:                true,
	})

	return nil
}
