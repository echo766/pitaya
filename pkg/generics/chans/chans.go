package chans

func IsClose[T any](c <-chan T) bool {
	select {
	case _, ok := <-c:
		return ok
	default:
		return false
	}
}

func IsFired[T any](c <-chan T) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

func TrySend[T any](c chan<- T, data T) bool {
	select {
	case c <- data:
		return true
	default:
		return false
	}
}
