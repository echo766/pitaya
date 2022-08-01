package actor

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/echo766/pitaya/constants"
	"github.com/echo766/pitaya/logger"
)

const (
	ASF_NULL    = iota
	ASF_RUN     = iota
	ASF_STOPPED = iota
	ASF_STOPING = iota
)

const MAIL_BOX_SIZE = 50
const TICK_INTERVAL = 100 * time.Millisecond

type (
	closeCallback func()
	tickCallback  func()

	jobRet struct {
		result interface{}
		err    error
	}

	job struct {
		fn   func() (interface{}, error)
		done chan jobRet
	}

	Actor interface {
		Init()
		Stop()
		Start()
		Exec(context.Context, func() (interface{}, error)) (interface{}, error)
		Push(context.Context, func() (interface{}, error)) error
		GetState() int32

		Actor() Actor
		OnClose(closeCallback)
		OnTick(tickCallback)
	}

	Impl struct {
		jobs     chan job
		state    int32
		stopSign chan bool
		stopped  chan bool
		closeFn  []closeCallback
		tickFn   []tickCallback
		tick     *time.Ticker
	}
)

func (a *Impl) Init() {
	a.jobs = make(chan job, MAIL_BOX_SIZE)
	a.stopSign = make(chan bool)
	a.stopped = make(chan bool)
	a.tick = time.NewTicker(TICK_INTERVAL)
}

func (a *Impl) Stop() {
	if atomic.CompareAndSwapInt32(&a.state, ASF_RUN, ASF_STOPING) {
		a.stopSign <- true
	}
}

func (a *Impl) Wait() {
	if atomic.CompareAndSwapInt32(&a.state, ASF_STOPING, ASF_STOPPED) {
		<-a.stopped
	}
}

func (a *Impl) Start() {
	if atomic.CompareAndSwapInt32(&a.state, ASF_NULL, ASF_RUN) {
		go a.run()
	}
}

func (a *Impl) Exec(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.Warnf("actor exec trace. %v", err)
		}
	}()

	if a.GetState() != ASF_RUN {
		return nil, constants.ErrActorStopped
	}

	done := make(chan jobRet)

	a.jobs <- job{fn: fn, done: done}

	select {
	case <-ctx.Done():
		return nil, constants.ErrHandleReqTimeout
	case ret := <-done:
		return ret.result, ret.err
	}
}

func (a *Impl) Push(ctx context.Context, fn func() (interface{}, error)) error {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.Warnf("actor exec trace. %v", err)
		}
	}()

	if a.GetState() != ASF_RUN {
		return constants.ErrActorStopped
	}

	a.jobs <- job{fn: fn, done: nil}
	return nil
}

func (a *Impl) GetState() int32 {
	return atomic.LoadInt32(&a.state)
}

func (a *Impl) setState(state int32) {
	atomic.StoreInt32(&a.state, state)
}

func (a *Impl) loop() bool {

	select {
	case <-a.stopSign:
		return true
	case j := <-a.jobs:
		a.consume(j)
	case <-a.tick.C:
		for _, v := range a.tickFn {
			a.callOnTick(v)
		}
	}

	return false
}

func (a *Impl) callOnTick(f tickCallback) {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.Warnf("actor tick callback trace. %w", err)
		}
	}()
	f()
}

func (a *Impl) callOnClose(f closeCallback) {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.Warnf("actor close callback trace. %w", err)
		}
	}()
	f()
}

func (a *Impl) run() {
	for {
		if a.loop() {
			break
		}
	}

	for _, v := range a.closeFn {
		a.callOnClose(v)
	}

	a.clear()
	a.stopped <- true
}

func (a *Impl) clear() {
	a.setState(ASF_NULL)
}

func (a *Impl) consume(j job) {
	var err error
	var ret interface{}
	defer func() {
		if j.done != nil {
			j.done <- jobRet{ret, err}
		}

		if err := recover(); err != nil {
			logger.Log.Warnf("actor call message trace.  %v", err)
		}
	}()

	ret, err = j.fn()
}

func (a *Impl) Actor() Actor {
	return a
}

func (a *Impl) OnClose(cb closeCallback) {
	a.closeFn = append(a.closeFn, cb)
}

func (a *Impl) OnTick(cb tickCallback) {
	a.tickFn = append(a.tickFn, cb)
}
