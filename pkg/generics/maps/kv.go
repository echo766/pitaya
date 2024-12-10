package maps

type Pair[K, V any] struct {
	Key   K
	Value V
}

func KV[K, V any](k K, v V) Pair[K, V] {
	return Pair[K, V]{
		Key:   k,
		Value: v,
	}
}
