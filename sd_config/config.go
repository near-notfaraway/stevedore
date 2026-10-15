package sd_config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/near-notfaraway/stevedore/sd_util"
)

type Config struct {
	Common  *CommonConfig
	PProf   *PProfConfig
	Log     *LogConfig
	Server  *ServerConfig
	Upload  *UploadConfig
	Session *SessionConfig
}

func Load(path string) (*Config, error) {
	config := new(Config)
	if err := sd_util.UnmarshalFile(path, config); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if config.Common == nil {
		return nil, fmt.Errorf("common config is required")
	}
	if err := config.TestCommon(); err != nil {
		return nil, fmt.Errorf("test common config: %w", err)
	}
	return config, nil
}

// ValidateUploadReload rejects changes outside the dynamically reloadable Upload section.
func ValidateUploadReload(current, next *Config) error {
	if current == nil {
		return fmt.Errorf("current config is required")
	}
	if next == nil {
		return fmt.Errorf("next config is required")
	}

	sections := []struct {
		name    string
		current any
		next    any
	}{
		{name: "Common", current: current.Common, next: next.Common},
		{name: "PProf", current: current.PProf, next: next.PProf},
		{name: "Log", current: current.Log, next: next.Log},
		{name: "Server", current: current.Server, next: next.Server},
		{name: "Session", current: current.Session, next: next.Session},
	}
	for _, section := range sections {
		if !reflect.DeepEqual(section.current, section.next) {
			return fmt.Errorf("non-Upload config changed: %s", section.name)
		}
	}
	return nil
}

// test common config before option handle finished
func (c *Config) TestCommon() error {
	// use project dir for default
	if c.Common.WorkingDir == "" {
		execPath, err := filepath.Abs(os.Args[0])
		if err != nil {
			return fmt.Errorf("get exec path failed: %s", err)
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

type CommonConfig struct {
	WorkingDir string // working dir
	PidPath    string // pid file
}

type PProfConfig struct {
	Open       bool   // if open pprof
	ServerAddr string // pprof server addr
}

type LogConfig struct {
	Path                   string // log file path
	Level                  string // log level
	Verbose                bool   // log caller information
	MaxAgeHour             int    // max age for clean up expired log
	RotationTimeHour       int    // time interval of rotating log
	UploadStatsIntervalSec int    // upload stats log interval; disabled when <= 0
}

type ServerConfig struct {
	ListenAddr          string // listening address
	ListenParallel      int    // requested reactors; Darwin currently uses one
	EventSize           int    // size of events polling from each selector
	BatchSize           int    // size of batch read/write packets
	BufSize             int    // size of single read/write buffer
	MaxRecvBatchesPerFD int    // max recvmmsg calls per fd in one reactor round
	MaxTryTimes         int    // max try times of upload packet to upstream
}

type UploadConfig struct {
	DefaultUpstream string
	Upstreams       []*UpstreamConfig
	Routes          []RouteConfig
}

type UpstreamConfig struct {
	Name             string
	Type             string
	KeyExtractor     *KeyExtractorConfig
	InvalidKeyPolicy string
	Peers            []*PeerConfig
	HealthChecker    *HealthCheckerConfig
}

type KeyExtractorConfig struct {
	Type        string
	Fixed       *FixedKeyConfig
	LengthField *LengthFieldKeyConfig
}

type FixedKeyConfig struct {
	KeyBytes string
	KeyMask  string
}

type LengthFieldKeyConfig struct {
	KeyStart     int
	LengthStart  int
	LengthBytes  int
	ByteOrder    string
	MaxKeyLength int
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
	Name       string
	Conditions []RouteConditionConfig
	Upstream   string
}

type RouteConditionConfig struct {
	Operator string
	Value    string
	Mask     string
	KeyBytes string
}

type SessionConfig struct {
	RecycleIntervalSec int64 // time interval of recycle session
	TimeoutSec         int64 // timeout for recycle session
}
