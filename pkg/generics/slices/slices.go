package slices

import (
	"github.com/echo766/pitaya/pkg/generics/constraints"
)

func Equal[E comparable](s1, s2 []E) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i, v := range s1 {
		if v != s2[i] {
			return false
		}
	}
	return true
}

func EqualFunc[E1, E2 any](s1 []E1, s2 []E2, eq func(E1, E2) bool) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i, v := range s1 {
		if !eq(v, s2[i]) {
			return false
		}
	}
	return true
}

func Compare[E constraints.Ordered](s1, s2 []E) int {
	s2len := len(s2)
	for i, v1 := range s1 {
		if i >= s2len {
			return 1
		}
		v2 := s2[i]
		switch {
		case v1 < v2:
			return -1
		case v1 > v2:
			return 1
		}
	}
	if len(s1) < s2len {
		return -1
	}
	return 0
}

func CompareFunc[E1, E2 any](s1 []E1, s2 []E2, cmp func(E1, E2) int) int {
	s2len := len(s2)
	for i, v1 := range s1 {
		if i >= s2len {
			return 1
		}
		v2 := s2[i]
		if c := cmp(v1, v2); c != 0 {
			return c
		}
	}
	if len(s1) < s2len {
		return -1
	}
	return 0
}

func Index[E comparable](s []E, e E) int {
	for i, v := range s {
		if v == e {
			return i
		}
	}
	return -1
}

func IndexFunc[E any](s []E, f func(E) bool) int {
	for i, v := range s {
		if f(v) {
			return i
		}
	}
	return -1
}

func Contains[E comparable](s []E, e E) bool {
	return Index(s, e) >= 0
}

func Insert[S ~[]E, E any](s S, i int, v ...E) S {
	tot := len(s) + len(v)
	if tot <= cap(s) {
		s2 := s[:tot]
		copy(s2[i+len(v):], s2[i:])
		copy(s2[i:], v)
		return s2
	}
	s2 := make(S, tot)
	copy(s2, s[:i])
	copy(s2[i:], v)
	copy(s2[i+len(v):], s[i:])
	return s2
}

func Delete[S ~[]E, E any](s S, i, j int) S {
	return append(s[:i], s[j:]...)
}

func Clone[S ~[]E, E any](s S) S {
	if s == nil {
		return nil
	}
	return append(S([]E{}), s...)
}

func Compact[S ~[]E, E comparable](s S) S {
	return CompactFunc(s, func(e1, e2 E) bool { return e1 == e2 })
}

func CompactFunc[S ~[]E, E any](s S, eq func(E, E) bool) S {
	if len(s) == 0 {
		return s
	}
	i := 1
	last := s[0]
	for _, v := range s[1:] {
		if !eq(v, last) {
			s[i] = v
			i++
			last = v
		}
	}
	return s[:i]
}

func Grow[S ~[]E, E any](s S, n int) S {
	return append(s, make(S, n)...)[:len(s)]
}

func Clip[S ~[]E, E any](s S) S {
	return s[:len(s):len(s)]
}

func Map[T1, T2 any](s []T1, f func(T1) T2) []T2 {
	r := make([]T2, len(s))
	for i, v := range s {
		r[i] = f(v)
	}
	return r
}

func Reduce[T1, T2 any](s []T1, initializer T2, f func(T2, T1) T2) T2 {
	r := initializer
	for _, v := range s {
		r = f(r, v)
	}
	return r
}

func Filter[T any](s []T, f func(T) bool) []T {
	var r []T
	for _, v := range s {
		if f(v) {
			r = append(r, v)
		}
	}
	return r
}
