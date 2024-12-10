package actor

import (
	"errors"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/echo766/pitaya/pkg/logger"
)

type (
	JobRet struct {
		result interface{}
		err    error
	}

	Job struct {
		name     string
		router   string
		fn       func() (interface{}, error)
		result   chan JobRet
		createAt int64
		run      atomic.Bool
	}
)

func NewJob(name, router string, fn func() (interface{}, error), wait bool) *Job {
	j := &Job{
		name:     name,
		router:   router,
		fn:       fn,
		createAt: time.Now().UnixMilli(),
	}
	if wait {
		j.result = make(chan JobRet, 1)
	}

	return j
}

func (j *Job) IsRunning() bool {
	return j.run.Load()
}

func (j *Job) Done() <-chan JobRet {
	return j.result
}

func (j *Job) Run() {
	defer func() {
		if err := recover(); err != nil {
			logger.Log.Warnf("actor run trace. %v stack: %s", err, string(debug.Stack()))

			if j.result != nil {
				j.result <- JobRet{result: nil, err: errors.New("tracing: job is canceled")}
			}
		}

		if j.result != nil {
			close(j.result)
		}
	}()

	j.run.Store(true)

	begin := time.Now().UnixMilli()
	defer func() {
		end := time.Now().UnixMilli()
		if end-begin > 1000 {
			logger.Log.WithFields(logger.Fields{
				"name":   j.name,
				"cost":   end - begin,
				"router": j.router,
			}).Warn("job took too long to finish")
		}
	}()

	runAt := time.Now().UnixMilli()
	if runAt-j.createAt > 500 {
		logger.Log.WithFields(logger.Fields{
			"name":   j.name,
			"cost":   runAt - j.createAt,
			"router": j.router,
		}).Warn("job took too long to start")
	}

	response, err := j.fn()
	if j.result != nil {
		j.result <- JobRet{result: response, err: err}
	}
}
