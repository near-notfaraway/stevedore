package sd_config

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

type Config struct {
	Common  *CommonConfig
	PProf   *PProfConfig
	Log     *LogConfig
	Server  *ServerConfig
	Upload  *UploadConfig
	Session *SessionConfig
}

type CommonConfig struct {
	WorkingDir string // working dir
	PidPath    string // pid file
}

type PProfConfig struct {
	Open       bool
	ServerAddr string
}

type LogConfig struct {
	Path             string // log file path
	Level            string // log level
	Verbose          bool   // log caller information
	MaxAgeHour       int    // max age for clean up expired log
	RotationTimeHour int    // time interval of rotating log
}

type ServerConfig struct {
	ListenAddr         string // listening address
	ListenParallel     int    // number of worker listening at the same time
	EventSize          int    // size of events polling from selector
	EventChanSize      int    // size of events delivering to worker non-blocking
	BatchSize          int    // size of batch read/write packets
	BufSize            int    // size of single read/write buffer
	TaskPoolSize       int    // capacity of task pool
	TaskPoolTimeoutSec int    // timeout of worker in task pool
	MaxTryTimes        int    // max try times of upload packet to upstream
}

type UploadConfig struct {
	DefaultUpstream string
	Upstreams       []*UpstreamConfig
	Routes          []RouteConfig
}

type UpstreamConfig struct {
	Name          string
	Type          string
	KeyBytes      string
	Peers         []*PeerConfig
	HealthChecker *HealthCheckerConfig
}

type PeerConfig struct {
	IP     string
	Port   int
	Weight int
	Backup bool
}

type HealthCheckerConfig struct {
	HeartbeatIntervalSec int
	HeartbeatTimeoutSec  int
	SuccessTimes         int
	FailedTimes          int
}

type RouteConfig struct {
	Operator string
	Value    string
	KeyBytes string
	Upstream string
}

type SessionConfig struct {
	RecycleIntervalSec int64 // time interval of recycle session
	TimeoutSec         int64 // timeout for recycle session
}

var GlobalConfig = new(Config)

func (c *Config) TestAndComplete() error {
	// use project dir for default
	if c.Common.WorkingDir == "" {
		execPath, err := filepath.Abs(os.Args[0])
		if err != nil {
			logrus.Fatalf("get exec path failed: %s", err)
		}
		c.Common.WorkingDir = filepath.Dir(filepath.Dir(execPath))
	}

	// use exec dir default
	if c.Common.PidPath == "" {
		execPath, err := filepath.Abs(os.Args[0])
		if err != nil {
			return fmt.Errorf("get exec path failed: %s", err)
		}
		c.Common.PidPath = filepath.Join(filepath.Dir(execPath), "stevedore.pid")
	}

	return nil
}