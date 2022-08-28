package sd_config

type Config struct {
	PProf   *PProfConfig
	Log     *LogConfig
	Server  *ServerConfig
	Upload  *UploadConfig
	Session *SessionConfig
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
	Upstreams []*UpstreamConfig
	Routes    []RouteConfig
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
}

type HealthCheckerConfig struct {
	HeartbeatIntervalSec int
	HeartbeatTimeoutSec  int
	SuccessTimes         int
	FailedTimes          int
}

type RouteConfig struct {
	Operator string
	Bytes    string
	Value    string
	Upstream string
}

type SessionConfig struct {
	RecycleIntervalSec int64 // time interval of recycle session
	TimeoutSec         int64 // timeout for recycle session
}
