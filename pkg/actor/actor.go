package actor

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/echo766/pitaya/pkg/async"
	"github.com/echo766/pitaya/pkg/constants"
	"github.com/echo766/pitaya/pkg/logger"
	"github.com/echo766/pitaya/pkg/route"
	"github.com/echo766/pitaya/pkg/timer"
)

const (
	ASF_NULL     = iota
	ASF_RUN      = iota
	ASF_STOPPED  = iota
	ASF_STOPPING = iota
)

const (
	MAIL_BOX_SIZE = 100
	//报警阈值
	INBOX_THRESHOLD = 30
	TICK_INTERVAL   = 500 * time.Millisecond
	EXEC_TIMEOUT    = 5 * time.Second
)

type (
	Event uint32

	closeCallback func()
	tickCallback  func()
	evtCallback   func(Event, interface{})

	Actor interface {
		Init()
		Start()
		Resume()
		Stop()
		Wait()
		Exec(context.Context, func() (interface{}, error)) (interface{}, error)
		Push(context.Context, func() (interface{}, error)) error
		GetState() int32
		SetName(string)

		Actor() Actor
		OnClose(closeCallback)
		OnTick(tickCallback)

		OnEvent(Event, evtCallback)
		EmitEvt(context.Context, Event, interface{})

		AddTimer(time.Duration, bool, func()) *timer.Timer
		EnableParallel() //并发执行任务
	}

	Impl struct {
		inbox      chan *Job
		state      int32
		stopSign   chan bool
		stopped    chan bool
		processing chan bool
		name       string
		// close
		closeFn     []closeCallback
		closeFnLock sync.RWMutex
		// event
		evtFn     map[Event][]evtCallback
		evtFnLock sync.RWMutex
		// parallel
		parallel      bool
		parallelJobId atomic.Int64
		parallelJob   sync.Map
		// tick
		ticker     *timer.Timer
		tickerLock sync.Mutex

		tickFnLock sync.RWMutex
		tickFn     []tickCallback
	}
)

func (a *Impl) Init() {
	a.inbox = make(chan *Job, MAIL_BOX_SIZE)
	a.stopSign = make(chan bool)
	a.stopped = make(chan bool)
	a.processing = make(chan bool)
	a.evtFn = make(map[Event][]evtCallback)
}

func (a *Impl) SetName(name string) {
	a.name = name
}

func (a *Impl) EnableParallel() {
	a.parallel = true
}

func (a *Impl) Stop() {
	if atomic.CompareAndSwapInt32(&a.state, ASF_RUN, ASF_STOPPING) {
		a.closeTick()
		a.stopSign <- true
	}
}

func (a *Impl) Resume() {
	a.processing <- true
}

func (a *Impl) Wait() {
	if atomic.CompareAndSwapInt32(&a.state, ASF_STOPPING, ASF_STOPPED) {
		<-a.stopped
	}
}

func (a *Impl) callTick() {
	a.tickFnLock.RLock()
	defer a.tickFnLock.RUnlock()

	for _, v := range a.tickFn {
		a.callOnTick(v)
	}
}

func (a *Impl) closeTick() {
	a.tickerLock.Lock()
	defer a.tickerLock.Unlock()

	if a.ticker != nil {
		a.ticker.Stop()
		a.ticker = nil
	}
}

func (a *Impl) startTick() {
	ticker := a.AddTimer(TICK_INTERVAL, false, func() {
		if a.GetState() != ASF_RUN {
			return
		}

		a.callTick()
		a.startTick()
	})

	a.tickerLock.Lock()
	defer a.tickerLock.Unlock()

	a.ticker = ticker

}

func (a *Impl) Start() {
	if atomic.CompareAndSwapInt32(&a.state, ASF_NULL, ASF_RUN) {
		async.GoRaw(func() {
			a.run()
		})
	}
}

func (a *Impl) getRouter(ctx context.Context) string {
	routeVal := ctx.Value(constants.RouteCtxKey)
	if routeVal == nil {
		return "nullroute"
	}
	return routeVal.(*route.Route).String()
}

func (a *Impl) Exec(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.WithFields(logger.Fields{
				"actor": a.name,
				"stack": string(debug.Stack()),
				"trace": err,
			}).Error("actor exec panic")
		}
	}()

	if a.GetState() != ASF_RUN {
		return nil, constants.ErrActorStopped
	}

	if len(a.inbox) >= INBOX_THRESHOLD {
		logger.Log.WithFields(logger.Fields{
			"actor": a.name,
			"queue": len(a.inbox),
		}).Warn("actor exec queue is too long")
	}

	beginTime := time.Now().Unix()
	rt := a.getRouter(ctx)
	job := NewJob(a.name, rt, fn, true)

	ctx, cancel := context.WithTimeout(ctx, EXEC_TIMEOUT)
	defer cancel()

	select {
	case a.inbox <- job:
	case <-ctx.Done():
		logger.Log.WithFields(logger.Fields{
			"actor":  a.name,
			"router": rt,
			"begin":  beginTime,
			"cost":   time.Now().Unix() - beginTime,
			"error":  ctx.Err(),
		}).Error("actor push job timeout on exec")

		return nil, constants.ErrHandleReqTimeout
	}

	select {
	case <-ctx.Done():
		logger.Log.WithFields(logger.Fields{
			"actor":   a.name,
			"router":  rt,
			"begin":   beginTime,
			"cost":    time.Now().Unix() - beginTime,
			"running": job.IsRunning(),
		}).Error("actor exec timeout on exec")

		return nil, constants.ErrHandleReqTimeout

	case result, ok := <-job.Done():
		if !ok {
			logger.Log.WithFields(logger.Fields{
				"actor":  a.name,
				"router": rt,
			}).Error("actor exec fail")

			return nil, constants.ErrHandleReqTimeout
		}
		return result.result, result.err
	}
}

func (a *Impl) Push(ctx context.Context, fn func() (interface{}, error)) error {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.WithFields(logger.Fields{
				"actor": a.name,
				"stack": string(debug.Stack()),
				"trace": err,
			}).Error("actor push panic")
		}
	}()

	if a.GetState() != ASF_RUN {
		return constants.ErrActorStopped
	}

	if len(a.inbox) >= INBOX_THRESHOLD {
		logger.Log.WithFields(logger.Fields{
			"actor": a.name,
			"queue": len(a.inbox),
		}).Warn("actor exec queue is too long")
	}

	rt := a.getRouter(ctx)
	newJob := NewJob(a.name, rt, fn, false)

	ctx, cancel := context.WithTimeout(ctx, EXEC_TIMEOUT)
	defer cancel()

	select {
	case a.inbox <- newJob:
	case <-ctx.Done():
		logger.Log.WithFields(logger.Fields{
			"actor":  a.name,
			"router": rt,
		}).Error("actor timeout on push")

		return constants.ErrHandleReqTimeout
	}

	return nil
}

func (a *Impl) EmitEvt(ctx context.Context, evt Event, data interface{}) {
	a.evtFnLock.RLock()
	defer a.evtFnLock.RUnlock()

	if len(a.evtFn[evt]) == 0 {
		return
	}

	a.Push(ctx, func() (interface{}, error) {
		a.evtFnLock.RLock()
		defer a.evtFnLock.RUnlock()

		for _, v := range a.evtFn[evt] {
			a.callOnEvt(v, evt, data)
		}
		return nil, nil
	})
}

func (a *Impl) GetState() int32 {
	return atomic.LoadInt32(&a.state)
}

func (a *Impl) SetState(state int32) {
	atomic.StoreInt32(&a.state, state)
}

func (a *Impl) loop() bool {

	select {
	case <-a.stopSign:
		return true
	case j := <-a.inbox:
		if a.parallel {
			a.asyncJob(j)
		} else {
			a.consume(j)
		}
	}

	return false
}

func (a *Impl) callOnTick(f tickCallback) {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.WithFields(logger.Fields{
				"actor": a.name,
				"stack": string(debug.Stack()),
				"trace": err,
			}).Error("actor tick callback panic")
		}
	}()
	f()
}

func (a *Impl) callOnClose(f closeCallback) {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.WithFields(logger.Fields{
				"actor": a.name,
				"stack": string(debug.Stack()),
				"trace": err,
			}).Error("actor close callback panic")
		}
	}()
	f()
}

func (a *Impl) callOnEvt(f evtCallback, evt Event, data interface{}) {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.WithFields(logger.Fields{
				"actor": a.name,
				"stack": string(debug.Stack()),
				"trace": err,
			}).Error("actor event callback panic")
		}
	}()
	f(evt, data)
}

func (a *Impl) run() {
	select {
	case <-a.stopSign:
	case <-a.processing:
	}
	if a.GetState() != ASF_RUN {
		return
	}

	a.startTick()

	for {
		if a.loop() {
			break
		}
	}
	defer func() {
		if err := recover(); err != nil {
			logger.Log.WithFields(logger.Fields{
				"actor": a.name,
				"stack": string(debug.Stack()),
				"trace": err,
			}).Error("actor run panic")
		}

		a.stopped <- true
	}()

	a.parallelJob.Range(func(key, value interface{}) bool {
		value.(*async.Job).Wait()
		return true
	})

	close(a.inbox)

	for j := range a.inbox {
		a.consume(j)
	}

	a.closeFnLock.RLock()
	for _, v := range a.closeFn {
		a.callOnClose(v)
	}
	a.closeFnLock.RUnlock()

}

func (a *Impl) consume(j *Job) {
	j.Run()
}

func (a *Impl) asyncJob(j *Job) {
	jobId := a.parallelJobId.Add(1)
	job := async.GoRaw(func() {
		a.consume(j)
		a.parallelJob.Delete(jobId)
	})

	a.parallelJob.Store(jobId, job)
}

func (a *Impl) Actor() Actor {
	return a
}

func (a *Impl) OnClose(cb closeCallback) {
	a.closeFnLock.Lock()
	defer a.closeFnLock.Unlock()

	a.closeFn = append(a.closeFn, cb)
}

func (a *Impl) OnTick(cb tickCallback) {
	a.tickFnLock.Lock()
	defer a.tickFnLock.Unlock()

	a.tickFn = append(a.tickFn, cb)
}

func (a *Impl) OnEvent(evt Event, cb evtCallback) {
	a.evtFnLock.Lock()
	defer a.evtFnLock.Unlock()

	a.evtFn[evt] = append(a.evtFn[evt], cb)
}

func (a *Impl) AddTimer(d time.Duration, repeat bool, fn func()) *timer.Timer {
	cb := func() {
		a.Push(context.Background(), func() (interface{}, error) {
			fn()
			return nil, nil
		})
	}

	if repeat {
		return timer.NewTimer(cb, d, timer.LoopForever)
	} else {
		return timer.NewTimer(cb, d, 1)
	}
}
