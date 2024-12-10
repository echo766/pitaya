package async

import (
	"context"

	"github.com/echo766/pitaya/pkg/generics/chans"
)

type Runner func()

func (f Runner) Run(ctx context.Context) {
	f()
}

func Parallel(ctx context.Context, fn ...interface {
	Run(context.Context)
},
) (<-chan struct{}, func()) {
	gg := NewGoGroup(GroupContext(ctx))
	for _, f := range fn {
		gg.Go(f.Run)
	}
	done := make(chan struct{})
	GoRaw(func() {
		gg.Wait()
		close(done)
	})
	return done, gg.Cancel
}

type parallelJob[T any] interface {
	Run(context.Context) (T, error)
}

type peer[T any] struct {
	chans.Result[T]
	task parallelJob[T]
}

func (p peer[T]) Run(ctx context.Context) {
	t, err := p.task.Run(ctx)
	if err != nil {
		p.Error(err)
		return
	}
	p.Send(t)
}

func Task[T any](task parallelJob[T]) peer[T] {
	p := peer[T]{
		Result: chans.MakeResult[T](),
		task:   task,
	}
	return p
}

type taskFunc[T any] func(context.Context) (T, error)

func (f taskFunc[T]) Run(ctx context.Context) (t T, err error) {
	if f == nil {
		return
	}
	return f(ctx)
}

func TaskFunc[T any](fn func(context.Context) (T, error)) peer[T] {
	p := peer[T]{
		Result: chans.MakeResult[T](),
		task:   taskFunc[T](fn),
	}
	return p
}

func Peer[T any](c chans.Result[T], fn func(context.Context) (T, error)) peer[T] {
	p := peer[T]{
		Result: c,
		task:   taskFunc[T](fn),
	}
	return p
}
