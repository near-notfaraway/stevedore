package sd_config

type Config struct {
	PProf *struct {
		Open       bool
		ServerAddr string
	}

	Log *LogConfig

	Server *struct {
		ListenAddr         string // listening address
		ListenParallel     int    // number of worker listening at the same time
		EventSize          int    // size of events polling from selector
		EventChanSize      int    // size of events delivering to worker non-blocking
		BatchSize          int    // size of batch read/write packets
		BufSize            int    // size of single read/write buffer
		TaskPoolSize       int    // capacity of task pool
		TaskPoolTimeoutSec int    // timeout of worker in task pool
		MaxTryTimes        int    // max try times for upload upstream
	}

	Session *SessionConfig
}

type LogConfig struct {
	Path             string // log file path
	Level            string // log level
	Verbose          bool   // log caller information
	MaxAgeHour       int    // max age for clean up expired log
	RotationTimeHour int    // time interval of rotating log
}

type SessionConfig struct {
	RecycleIntervalSec int64 // time interval of recycle session
	TimeoutSec         int64 // timeout for recycle session
}
