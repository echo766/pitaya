// Copyright (c) nano Author and TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package timer

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/echo766/pitaya/pkg/logger"
	pq "github.com/emirpasic/gods/queues/priorityqueue"
)

const (
	// LoopForever is a constant indicating that timer should loop forever
	LoopForever = -1
)

var (
	// Manager manager for all Timers
	Manager = &struct {
		queue *pq.Queue
		qlock sync.Mutex

		incrementID int64 // auto increment id
	}{}

	// Precision indicates the precision of timer, default is time.Second
	Precision = time.Second

	// GlobalTicker represents global ticker that all cron job will be executed
	// in globalTicker.
	GlobalTicker *time.Ticker
)

type (
	// Func represents a function which will be called periodically in main
	// logic gorontine.
	Func func()

	// Condition represents a checker that returns true when cron job needs
	// to execute
	Condition interface {
		Check(now time.Time) bool
	}

	// Timer represents a cron job
	Timer struct {
		ID       int64         // timer id
		fn       Func          // function that execute
		createAt int64         // timer create time
		interval time.Duration // execution interval
		elapse   int64         // total elapse time
		closed   atomic.Bool   // is timer closed
		counter  int           // counter
		lock     sync.Mutex    // lock
	}
)

func init() {
	Manager.queue = pq.NewWith(func(a, b interface{}) int {
		t1, t2 := a.(*Timer), b.(*Timer)
		if t1.createAt+t1.elapse < t2.createAt+t2.elapse {
			return -1
		} else if t1.createAt+t1.elapse > t2.createAt+t2.elapse {
			return 1
		}
		return 0
	})
}

// NewTimer creates a cron job
func NewTimer(fn Func, interval time.Duration, counter int) *Timer {
	id := atomic.AddInt64(&Manager.incrementID, 1)
	t := &Timer{
		ID:       id,
		fn:       fn,
		createAt: time.Now().UnixNano(),
		interval: interval,
		elapse:   int64(interval), // first execution will be after interval
		counter:  counter,
	}

	Manager.qlock.Lock()
	defer Manager.qlock.Unlock()

	// add to queue
	Manager.queue.Enqueue(t)

	return t
}

// Stop turns off a timer. After Stop, fn will not be called forever
func (t *Timer) Stop() {
	if t.closed.Load() {
		return
	}
	t.closed.Store(true)
	t.ResetFn()
}

// IsStoped returns true if timer is closed
func (t *Timer) IsStoped() bool {
	return t.closed.Load()
}

func (t *Timer) GetFn() Func {
	t.lock.Lock()
	defer t.lock.Unlock()
	fn := t.fn
	return fn
}

func (t *Timer) ResetFn() {
	t.lock.Lock()
	defer t.lock.Unlock()
	t.fn = nil
}

// execute job function with protection
func pexec(id int64, fn Func) {
	if fn == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			logger.Log.Errorf("Call timer function error, TimerID=%d, Error=%v", id, err)
		}
	}()

	fn()
}

func poptimer() *Timer {
	now := time.Now()
	unn := now.UnixNano()

	Manager.qlock.Lock()
	defer Manager.qlock.Unlock()

	for {
		ti, ok := Manager.queue.Peek()
		if !ok {
			return nil
		}
		t := ti.(*Timer)

		if t.IsStoped() {
			Manager.queue.Dequeue()
			continue
		}

		if t.counter == 0 {
			t.Stop()
			Manager.queue.Dequeue()
			continue
		}

		if t.createAt+t.elapse > unn {
			return nil
		}

		Manager.queue.Dequeue()
		return t
	}
}

func afterexec(t *Timer) {
	Manager.qlock.Lock()
	defer Manager.qlock.Unlock()

	if t.IsStoped() {
		return
	}
	t.elapse += int64(t.interval)

	// update timer counter
	if t.counter != LoopForever && t.counter > 0 {
		t.counter--
	}

	if t.counter == 0 {
		t.Stop()
		return
	}

	Manager.queue.Enqueue(t)
}

// Cron executes scheduled tasks
func Cron() {
	for {
		t := poptimer()
		if t == nil {
			return
		}

		pexec(t.ID, t.GetFn())
		afterexec(t)
	}
}
