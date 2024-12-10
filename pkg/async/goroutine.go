package async

import (
	"container/list"
	"context"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/echo766/pitaya/pkg/logger/interfaces"
)

var (
	panicLogger    interfaces.Logger
	defaultContext = context.TODO()
)

// SetGoPanicLogger for setting goroutine panic logger
func SetGoPanicLogger(l interfaces.Logger) {
	panicLogger = l
}

func SetDefaultContext(ctx context.Context) {
	defaultContext = ctx
}

// RestartPolicy number and interval of restarts
type RestartPolicy struct {
	// If Times is 0, it will not restart
	Times int
	// Default restart delay time, if the call to DelayFunc fails this value is still used
	Delay time.Duration
	// If DelayFunc is set, DelayFunc is used first, otherwise Delay is used.
	// If the second return value is false, it will not restart
	DelayFunc func(int) (time.Duration, bool)

	restartTimes int
}

type (
	options struct {
		wg      *sync.WaitGroup
		timeout time.Duration
		ctx     context.Context
		// blockChan is used to control the number of goroutines
		blockChan chan struct{}
		// pre-execution callbacks
		preFunc func()
		// post-execution callbacks
		postFunc func()
		// finalFunc will call back without panic
		// its panic cannot be recovered, so it cannot be set by external package
		finalFunc func(error)
		// a custom job，cannot be set by external package
		job *Job
		// whether to restart the goroutine after crash
		restartPolicy RestartPolicy
		// count of goroutines
		count uint64
	}
	goWaitGroup struct {
		*sync.WaitGroup
	}
	goTimeout time.Duration
	goCount   uint64
	goContext struct {
		context.Context
	}
	goBlockChan chan struct{}
	goPreFunc   func()
	goPostFunc  func()
	goFinalFunc func(error)
)

// Option pattern
type Option interface {
	apply(*options)
}

func (wg goWaitGroup) apply(o *options) {
	o.wg = wg.WaitGroup
}

// WithWaitGroup for set external waitgroup
func WithWaitGroup(wg *sync.WaitGroup) Option {
	return goWaitGroup{wg}
}

func (c goCount) apply(o *options) {
	o.count = uint64(c)
}

// WithCount for set repeat count of goroutine
func WithCount(c uint64) Option {
	return goCount(c)
}

func (ctx goContext) apply(o *options) {
	o.ctx = ctx.Context
}

// WithContext for custom context
func WithContext(ctx context.Context) Option {
	return goContext{ctx}
}

func (c goBlockChan) apply(o *options) {
	o.blockChan = c
}

// WithBlockChan for concurrent count control
func WithBlockChan(c chan struct{}) Option {
	return goBlockChan(c)
}

func (t goTimeout) apply(o *options) {
	o.timeout = time.Duration(t)
}

// WithTimeout for maximum execution time
func WithTimeout(d time.Duration) Option {
	return goTimeout(d)
}

func (f goPreFunc) apply(o *options) {
	o.preFunc = f
}

// OnPre to set pre-execution callbacks
func OnPre(f func()) Option {
	return goPreFunc(f)
}

func (f goPostFunc) apply(o *options) {
	o.postFunc = f
}

// OnPost to set post-execution callbacks
func OnPost(f func()) Option {
	return goPostFunc(f)
}

func (r RestartPolicy) apply(o *options) {
	o.restartPolicy = r
}

// RestartOnFailure to set the policy to restart the goroutine after crash
func RestartOnFailure(r RestartPolicy) Option {
	return r
}

// AlwaysRestartOnFailure is used to restart the job after crash
func AlwaysRestartOnFailure() Option {
	return RestartPolicy{
		DelayFunc: func(i int) (time.Duration, bool) {
			return 0, true
		},
	}
}

func (j *Job) apply(o *options) {
	o.job = j
}

func withJob(j *Job) Option {
	return j
}

func (f goFinalFunc) apply(o *options) {
	o.finalFunc = f
}

func onFinal(f func(error)) Option {
	return goFinalFunc(f)
}

func (opt options) apply(o *options) {
	*o = opt
}

func withOption(o options) Option {
	return o
}

// Job is a goroutine controller
type Job struct {
	started bool
	ctx     context.Context
	cancel  context.CancelFunc

	count    uint64
	err      error
	doneLock sync.Mutex
	stopped  bool
	done     chan struct{}
}

// Err is any error during job
func (j *Job) Err() error {
	return j.err
}

// Cancel is used to try to stop a goroutine
func (j *Job) Cancel() {
	if !j.started {
		j.complete()
	}
	j.cancel()
}

func (j *Job) completeOne() {
	if atomic.AddUint64(&j.count, ^uint64(0)) == 0 {
		j.complete()
	}
}

// complete ensure that the function can be called multiple times
// without duplicating the close channel
func (j *Job) complete() {
	j.doneLock.Lock()
	if !j.stopped {
		close(j.done)
		j.stopped = true
	}
	j.doneLock.Unlock()
}

// Wait for goroutine complete
func (j *Job) Wait() {
	<-j.done
}

// Stop is cancel and wait
func (j *Job) Stop() {
	j.cancel()
	<-j.done
}

// Done is then channel for job complete
func (j *Job) Done() <-chan struct{} {
	return j.done
}

// Stopped is used to check if the job is stopped
func (j *Job) Stopped() bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

func newJob(ctx context.Context, cancel context.CancelFunc, c uint64) *Job {
	j := &Job{
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
		count:  c,
	}
	return j
}

func newErrJob(err error) *Job {
	j := &Job{
		cancel: func() {},
		done:   make(chan struct{}),
		err:    err,
	}
	close(j.done)
	return j
}

// GoWithErr protected goroutine startup method with error return
func GoWithErr(f func(context.Context) error, opts ...Option) *Job {
	o := options{
		ctx:   defaultContext,
		count: 1,
	}
	for _, opt := range opts {
		opt.apply(&o)
	}

	// if no external job is passed in, initialize one
	if o.job == nil {
		var cancel context.CancelFunc
		if o.timeout != 0 {
			o.ctx, cancel = context.WithTimeout(o.ctx, o.timeout)
		} else {
			o.ctx, cancel = context.WithCancel(o.ctx)
		}
		o.job = newJob(o.ctx, cancel, o.count)
	}

	// mark as started, so that any closures before that can be done immediately
	// without having to wait until the function is actually dispatched
	// even if the true set here is not read due to concurrency,
	// the done will be triggered soon after, so there will be no problem
	o.job.started = true

	// if the job has been cancelled externally, it is returned directly
	select {
	case <-o.job.ctx.Done():
		if o.finalFunc != nil {
			// the final func is not set externally, so panic cases are not considered
			go o.finalFunc(o.job.err)
		}
		o.job.complete()
		return o.job
	default:
	}

	if o.wg != nil {
		o.wg.Add(int(o.count))
	}

	var fn func()
	fn = func() {
		// if there is a limit on the number of goroutines that can run simultaneously,
		// determine if they can run first
		if o.blockChan != nil {
			o.blockChan <- struct{}{}
		}
		delay, restart := o.restartPolicy.Delay, o.restartPolicy.Times != 0 && o.restartPolicy.Times > o.restartPolicy.restartTimes
		defer func() {
			complete := true
			if err := recover(); err != nil {
				if panicLogger != nil {
					buf := make([]byte, 2048)
					n := runtime.Stack(buf, false)
					panicLogger.Errorf("%+v\n", err)
					panicLogger.Errorf(string(buf[:n]))
				}
				if restart {
					o.restartPolicy.restartTimes++
					time.Sleep(delay)
					go fn()
					complete = false
				}
			}
			// The following actions are to ensure that no crashes occur
			if o.blockChan != nil {
				<-o.blockChan
			}
			if complete {
				if o.finalFunc != nil {
					o.finalFunc(o.job.err)
				}
				o.job.completeOne()
				if o.wg != nil {
					o.wg.Done()
				}
			}
		}()
		// prevent DelayFunc from crashing and causing the process to exit
		if o.restartPolicy.DelayFunc != nil {
			delay, restart = o.restartPolicy.DelayFunc(o.restartPolicy.restartTimes)
		}
		if o.preFunc != nil {
			o.preFunc()
		}
		o.job.err = f(o.job.ctx)
		if o.postFunc != nil {
			o.postFunc()
		}
	}

	for i := uint64(0); i < o.count; i++ {
		go fn()
	}

	return o.job
}

// Go protected goroutine startup method
func Go(f func(context.Context), opts ...Option) *Job {
	return GoWithErr(func(ctx context.Context) error {
		f(ctx)
		return nil
	}, opts...)
}

// GoRaw start a goroutine with panic protection only
func GoRaw(f func(), opts ...Option) *Job {
	return GoWithErr(func(context.Context) error {
		f()
		return nil
	}, opts...)
}

// GoGroup is a goroutine wrapper that limits the number of concurrent and generated
type GoGroup struct {
	ctx    context.Context
	cancel context.CancelFunc

	blockChan    chan struct{}
	maxGoroutine uint64

	lock             sync.Mutex
	currentGoroutine uint64
	waitJobs         list.List

	wg sync.WaitGroup

	err         atomic.Value
	errorPolicy ErrorPolicy
}

type element struct {
	job     *Job
	options options
	count   uint64
	f       func(context.Context) error
}

// GoRaw start a goroutine ignore context
func (gg *GoGroup) GoRaw(f func(), opts ...Option) *Job {
	return gg.GoWithErr(func(context.Context) error {
		f()
		return nil
	}, opts...)
}

// Go start a goroutine with strategy
func (gg *GoGroup) Go(f func(context.Context), opts ...Option) *Job {
	return gg.GoWithErr(func(ctx context.Context) error {
		f(ctx)
		return nil
	}, opts...)
}

func (gg *GoGroup) tryGo() {
	getNext := func() (*element, bool) {
		gg.lock.Lock()
		defer gg.lock.Unlock()
		if gg.currentGoroutine >= gg.maxGoroutine {
			return nil, false
		}
		el := gg.waitJobs.Front()
		if el == nil {
			return nil, false
		}
		e := el.Value.(*element)
		e.count--
		if e.count == 0 || e.job.Stopped() {
			gg.waitJobs.Remove(el)
		}
		gg.currentGoroutine++
		return e, true
	}
	for {
		e, ok := getNext()
		if !ok {
			break
		}
		GoWithErr(e.f, withOption(e.options), WithBlockChan(gg.blockChan), WithContext(gg.ctx),
			WithWaitGroup(&gg.wg),
			onFinal(gg.onFinal), withJob(e.job))
	}
}

// GoWithErr start a goroutine with strategy and return error
func (gg *GoGroup) GoWithErr(f func(context.Context) error, opts ...Option) *Job {
	err := gg.Err()
	if err != nil && gg.errorPolicy == ErrorPolicyStop {
		return newErrJob(err)
	}
	o := options{
		count: 1,
	}
	for _, opt := range opts {
		opt.apply(&o)
	}
	ctx, cancel := context.WithCancel(gg.ctx)
	// the number of tasks started is handled here, and the number of tasks passed to GoWithErr is always 1
	count := o.count
	o.count = 1
	job := newJob(ctx, cancel, count)
	// if maxGoroutine is 0, then there is no limit to the number of goroutines

	if gg.maxGoroutine != 0 {
		e := &element{
			job:     job,
			f:       f,
			count:   count,
			options: o,
		}
		gg.lock.Lock()
		gg.waitJobs.PushBack(e)
		gg.lock.Unlock()
		gg.tryGo()
		return job
	}
	GoWithErr(f, withOption(o), WithBlockChan(gg.blockChan), withJob(job),
		WithWaitGroup(&gg.wg), onFinal(gg.onFinal))
	return job
}

// when a goroutine is finished, determine if a new goroutine is needed
func (gg *GoGroup) onFinal(err error) {
	if gg.Err() != nil && gg.errorPolicy == ErrorPolicyStop {
		return
	}
	if err != nil {
		gg.err.Store(err)
		if gg.errorPolicy == ErrorPolicyStop {
			gg.cancel()
		}
		return
	}
	if gg.maxGoroutine == 0 {
		return
	}
	gg.lock.Lock()
	gg.currentGoroutine--
	gg.lock.Unlock()
	gg.tryGo()
}

// Cancel is used to try to stop goroutine
func (gg *GoGroup) Cancel() {
	gg.lock.Lock()
	gg.waitJobs.Init()
	gg.lock.Unlock()
	gg.cancel()
}

// Err get first error of goroutines
func (gg *GoGroup) Err() error {
	err, ok := gg.err.Load().(error)
	if !ok {
		return nil
	}
	return err
}

// Wait for all job complete
func (gg *GoGroup) Wait() {
	gg.wg.Wait()
}

type (
	groupOptions struct {
		ctx              context.Context
		goroutineCount   uint64
		concurrencyCount uint64
		blockChan        chan struct{}
		errorPolicy      ErrorPolicy
	}
	goGroupContext struct {
		context.Context
	}
	goGroupGouroutineCount  uint64
	goGroupConcurrencyCount uint64
	goGroupBlockChan        chan struct{}
	goGroupErrorPolicy      ErrorPolicy
)

// GroupOption is option pattern for NewGoGroup
type GroupOption interface {
	apply(*groupOptions)
}

func (gc goGroupGouroutineCount) apply(g *groupOptions) {
	g.goroutineCount = uint64(gc)
}

// MaxGoroutine is used to set max goroutine count
func MaxGoroutine(c uint64) GroupOption {
	return goGroupGouroutineCount(c)
}

func (cc goGroupConcurrencyCount) apply(g *groupOptions) {
	g.concurrencyCount = uint64(cc)
	g.blockChan = make(chan struct{}, cc)
}

// MaxConcurrency is used to set max concurrency count
func MaxConcurrency(c uint64) GroupOption {
	return goGroupConcurrencyCount(c)
}

func (ctx goGroupContext) apply(g *groupOptions) {
	g.ctx = ctx.Context
}

// GroupContext is used to set external context
func GroupContext(ctx context.Context) GroupOption {
	return goGroupContext{ctx}
}

// ErrorPolicy for what to do if you encounter an error
type ErrorPolicy = int8

const (
	// ErrorPolicyStop will not continue with remaining goroutine
	ErrorPolicyStop ErrorPolicy = iota + 1
	// ErrorPolicyCountine will ignore error
	ErrorPolicyCountine
)

func (policy goGroupErrorPolicy) apply(g *groupOptions) {
	g.errorPolicy = ErrorPolicy(policy)
}

// GroupErrorPolicy set what to do if you encounter an error
func GroupErrorPolicy(policy ErrorPolicy) GroupOption {
	return goGroupErrorPolicy(policy)
}

// NewGoGroup is used to create a GoGroup
func NewGoGroup(opts ...GroupOption) *GoGroup {
	o := groupOptions{
		ctx:         defaultContext,
		errorPolicy: ErrorPolicyCountine,
	}
	for _, opt := range opts {
		opt.apply(&o)
	}
	// the number of concurrency cannot exceed the number of goroutines
	if o.goroutineCount != 0 && o.concurrencyCount > o.goroutineCount {
		o.concurrencyCount = o.goroutineCount
		o.blockChan = make(chan struct{}, o.concurrencyCount)
	}
	ctx, cancel := context.WithCancel(o.ctx)
	return &GoGroup{
		ctx:          ctx,
		cancel:       cancel,
		maxGoroutine: o.goroutineCount,
		blockChan:    o.blockChan,
		errorPolicy:  o.errorPolicy,
	}
}

// GoID returns id of goroutine if exists, otherwise return 0
func GoID() int {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	idField := strings.Fields(strings.TrimPrefix(string(buf[:n]), "goroutine "))[0]
	id, _ := strconv.Atoi(idField)
	return id
}
