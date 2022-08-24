package sd_config

type Config struct {
	PProf *struct {
		Open       bool
		ServerAddr string
	}

	Server *struct {
		ListenAddr         string // listening address
		ListenParallel     int    // number of worker listening at the same time
		EventSize          int    // size of events polling from selector
		EventChanSize      int    // size of events delivering to worker non-blocking
		BatchSize          int    // size of batch read/write packets
		BufSize            int    // size of single read/write buffer
		TaskPoolSize       int    // capacity of task pool
		TaskPoolTimeoutSec int    // timeout of worker in task pool
	}
}
