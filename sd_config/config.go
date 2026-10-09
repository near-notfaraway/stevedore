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
	ListenAddr          string // listening address
	ListenParallel      int    // number of independent reactors
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
