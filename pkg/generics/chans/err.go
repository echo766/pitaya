package chans

type (
	result[T any] struct {
		data T
		err  error
	}

	Result[T any] chan interface {
		Data() T
		Error() error
	}
)

func (t result[T]) Data() T {
	return t.data
}

func (t result[T]) Error() error {
	return t.err
}

func (c Result[T]) Send(data T) {
	c <- result[T]{
		data: data,
	}
}

func (c Result[T]) Error(err error) {
	c <- result[T]{
		err: err,
	}
}

func MakeResult[T any]() Result[T] {
	return make(Result[T])
}

func MakeBufferResult[T any](size int) Result[T] {
	return make(Result[T], size)
}
