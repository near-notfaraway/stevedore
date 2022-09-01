package sd_util

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// should be finished when very few task
func TestSimpleTaskPool_NewTask1(t *testing.T) {
	var finished int64
	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		taskPool := NewSimpleTaskPool(0, 10)
		wg.Add(1)
		taskPool.Go(func() {
			atomic.AddInt64(&finished, 1)
			wg.Done()
		})
		if taskPool.WorkerCount() < 1 {
			t.Errorf("worker start failed\n")
		}
	}

	wg.Wait()
	if finished != 100 {
		t.Errorf("not all tasks are completed\n")
	}
}

// should be recycle when task is empty
func TestSimpleTaskPool_NewTask2(t *testing.T) {
	var finished int64
	wg := sync.WaitGroup{}
	taskPool := NewSimpleTaskPool(0, 10)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		taskPool.Go(func() {
			atomic.AddInt64(&finished, 1)
			wg.Done()
		})
		if taskPool.WorkerCount() < 1 {
			t.Errorf("worker start failed\n")
		}
	}

	wg.Wait()
	if finished != 100 {
		t.Errorf("not all tasks are completed\n")
	}

	time.Sleep(time.Second * 11)
	if taskPool.WorkerCount() > 0 {
		t.Errorf("workerNum recycle failed\n")
	}
}

// should be safe when run concurrently
func TestSimpleTaskPool_NewTask3(t *testing.T) {
	var finished int64
	wg := sync.WaitGroup{}
	taskPool := NewSimpleTaskPool(0, 10)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			taskPool.Go(func() {
				atomic.AddInt64(&finished, 1)
				wg.Done()
			})
			if taskPool.WorkerCount() < 1 {
				t.Errorf("worker start failed\n")
			}
		}()
	}

	wg.Wait()
	if finished != 100 {
		t.Errorf("not all tasks are completed\n")
	}

	time.Sleep(time.Second * 11)
	if taskPool.WorkerCount() > 0 {
		t.Errorf("workerNum recycle failed\n")
	}
}

// benchmark about add task to task pool
func BenchmarkNewSimpleTaskPool1(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	wg := sync.WaitGroup{}
	taskPool := NewSimpleTaskPool(0, 10)

	for i := 0; i < b.N; i++ {
		wg.Add(1)
		taskPool.Go(func() {
			wg.Done()
		})
	}
	wg.Wait()
}
