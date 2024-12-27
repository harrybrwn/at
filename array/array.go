package array

import (
	"fmt"
	"iter"
)

func Append[T any](arrs ...[]T) []T {
	res := make([]T, 0)
	for _, arr := range arrs {
		res = append(res, arr...)
	}
	return res
}

func Reverse[T any](s []T) {
	l := len(s)
	m := l / 2
	j := 0
	for i := 0; i < m; i++ {
		j = l - i - 1
		s[i], s[j] = s[j], s[i]
	}
}

func Map[I, O any](arr []I, fn func(I) O) []O {
	res := make([]O, len(arr))
	for i, v := range arr {
		res[i] = fn(v)
	}
	return res
}

func MapStringers[Slice ~[]T, T fmt.Stringer](s Slice) []string {
	return Map(s, func(e T) string { return e.String() })
}

func ToAny[T any](v T) any { return v }

func FilterMap[I, O any](in iter.Seq[I], fn func(*I) (O, bool)) iter.Seq[O] {
	return func(yield func(O) bool) {
		for item := range in {
			o, ok := fn(&item)
			if ok && !yield(o) {
				return
			}
		}
	}
}

func Iter[T any](s []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, v := range s {
			if !yield(v) {
				return
			}
		}
	}
}

func IterRef[T any](s []T) iter.Seq[*T] {
	return func(yield func(*T) bool) {
		for i := range s {
			if !yield(&s[i]) {
				return
			}
		}
	}
}

func IterDeref[T any](s []*T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := range s {
			v := *s[i]
			if !yield(v) {
				return
			}
		}
	}
}

func Move[T any](array []T, srcIndex int, dstIndex int) []T {
	value := array[srcIndex]
	return insert(remove(array, srcIndex), value, dstIndex)
}

func MoveToFront[T any](arr []T, fn func(T) bool) {
	for i := 0; i < len(arr); i++ {
		if fn(arr[i]) {
			Move(arr, i, 0)
			return
		}
	}
}

func insert[T any](array []T, value T, index int) []T {
	return append(array[:index], append([]T{value}, array[index:]...)...)
}

func remove[T any](array []T, index int) []T {
	return append(array[:index], array[index+1:]...)
}

func Remove[T any](index int, arr []T) []T {
	l := len(arr) - 1
	arr[index] = arr[l]
	return arr[:l]
}
