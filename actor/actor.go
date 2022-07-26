package actor

import (
	"context"
	"sync/atomic"

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

type (
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
	}

	Impl struct {
		jobs     chan job
		state    int32
		stopSign chan bool
		stopped  chan bool
	}
)

func (a *Impl) Init() {
	a.jobs = make(chan job, MAIL_BOX_SIZE)
	a.stopSign = make(chan bool)
	a.stopped = make(chan bool)
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
	}

	return false
}

func (a *Impl) run() {
	for {
		if a.loop() {
			break
		}
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
