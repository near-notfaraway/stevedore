package sd_selector

import (
	"time"
)

const (
	DefaultTaskPoolSize       = 1024
	DefaultTaskPoolTimeoutSec = 10
)

type TaskPool interface {
	Go(task func())
	WorkerCount() int
}

//------------------------------------------------------------------------------
// SimpleTaskPool: a safe task pool without mutex
// - tasks that exceed capacity will block
// - workers can recycle automatically
//------------------------------------------------------------------------------

type SimpleTaskPool struct {
	task    chan func()   // channel for submit task
	sem     chan struct{} // semaphore for limit number of workers
	timeout time.Duration // timeout for recycle worker
}

func NewSimpleTaskPool(size, timeout int) TaskPool {
	if size < 1 {
		size = DefaultTaskPoolSize
	}

	if timeout < DefaultTaskPoolTimeoutSec {
		timeout = DefaultTaskPoolTimeoutSec
	}

	return &SimpleTaskPool{
		task:    make(chan func()),
		sem:     make(chan struct{}, size),
		timeout: time.Second * time.Duration(timeout),
	}
}

func (p *SimpleTaskPool) Go(task func()) {
	select {
	case p.task <- task:
	case p.sem <- struct{}{}:
		go p.worker(task)
	}
}

func (p *SimpleTaskPool) worker(task func()) {
	defer func() { <-p.sem }()
	timer := time.NewTimer(p.timeout)
	task()

	for {
		select {
		case <-timer.C:
			return
		case task = <-p.task:
			timer.Reset(p.timeout)
			task()
		}
	}
}

func (p *SimpleTaskPool) WorkerCount() int {
	return len(p.sem)
}
