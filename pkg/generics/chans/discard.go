package chans

func Discard[T any]() chan T {
	tc := make(chan T)
	go func() {
		for range tc {
		}
	}()
	return tc
}

type anyDone interface {
	Done()
}

func DiscardDone[T anyDone]() chan T {
	tc := make(chan T)
	go func() {
		f := func(t T) {
			defer recover()
			t.Done()
		}
		for t := range tc {
			f(t)
		}
	}()
	return tc
}
