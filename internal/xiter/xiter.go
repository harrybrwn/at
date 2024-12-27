// See https://github.com/golang/go/issues/61898
package xiter

import (
	"cmp"
	"fmt"
	"iter"
)

// Filter2 returns an iterator over seq that only includes
// the pairs k, v for which f(k, v) is true.
func Filter2[K, V any](f func(K, V) bool, seq iter.Seq2[K, V]) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for k, v := range seq {
			if f(k, v) && !yield(k, v) {
				return
			}
		}
	}
}

type Pair[K, V any] struct {
	K K
	V V
}

type Group[K, V any] struct {
	Key   string
	Pairs []Pair[K, V]
}

func GroupBy2[K, V any](seq iter.Seq2[K, V], f func(K, V) string) []Group[K, V] {
	groups := make(map[string][]Pair[K, V])
	for k, v := range seq {
		key := f(k, v)
		if pairs, ok := groups[key]; ok {
			groups[key] = append(pairs, Pair[K, V]{K: k, V: v})
		} else {
			groups[key] = []Pair[K, V]{{K: k, V: v}}
		}
	}
	pairs := make([]Group[K, V], 0)
	for k, group := range groups {
		pairs = append(pairs, Group[K, V]{Key: k, Pairs: group})
	}
	return pairs
}

// Map returns an iterator over f applied to seq.
func Map[In, Out any](f func(In) Out, seq iter.Seq[In]) iter.Seq[Out] {
	return func(yield func(Out) bool) {
		for in := range seq {
			if !yield(f(in)) {
				return
			}
		}
	}
}

// Map2 returns an iterator over f applied to seq.
func Map2[KIn, VIn, KOut, VOut any](f func(KIn, VIn) (KOut, VOut), seq iter.Seq2[KIn, VIn]) iter.Seq2[KOut, VOut] {
	return func(yield func(KOut, VOut) bool) {
		for k, v := range seq {
			if !yield(f(k, v)) {
				return
			}
		}
	}
}

func Keys[K, V any](s iter.Seq2[K, V]) iter.Seq[K] {
	return func(yield func(K) bool) {
		for k := range s {
			if !yield(k) {
				return
			}
		}
	}
}

func Vals[K, V any](s iter.Seq2[K, V]) iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range s {
			if !yield(v) {
				return
			}
		}
	}
}

// Chunk returns an iterator over consecutive slices of up to n elements of seq.
// All but the last slice will have size n.
// All slices are clipped to have no capacity beyond the length.
// If seq is empty, the sequence is empty: there is no empty slice in the sequence.
// Chunk panics if n is less than 1.
func Chunk[E any](seq iter.Seq[E], n int) iter.Seq[[]E] {
	if n < 1 {
		panic("cannot be less than 1")
	}

	return func(yield func([]E) bool) {
		var batch []E

		for e := range seq {
			if batch == nil {
				batch = make([]E, 0, n)
			}

			batch = append(batch, e)

			if len(batch) == n {
				if !yield(batch) {
					return
				}

				batch = nil
			}
		}

		if l := len(batch); l > 0 {
			yield(batch[:l:l])
		}
	}
}

// Merge merges two sequences of ordered values.
// Values appear in the output once for each time they appear in x
// and once for each time they appear in y.
// If the two input sequences are not ordered,
// the output sequence will not be ordered,
// but it will still contain every value from x and y exactly once.
//
// Merge is equivalent to calling MergeFunc with cmp.Compare[V]
// as the ordering function.
func Merge[V cmp.Ordered](x, y iter.Seq[V]) iter.Seq[V] {
	return MergeFunc(x, y, cmp.Compare[V])
}

// MergeFunc merges two sequences of values ordered by the function f.
// Values appear in the output once for each time they appear in x
// and once for each time they appear in y.
// When equal values appear in both sequences,
// the output contains the values from x before the values from y.
// If the two input sequences are not ordered by f,
// the output sequence will not be ordered by f,
// but it will still contain every value from x and y exactly once.
func MergeFunc[V any](x, y iter.Seq[V], f func(V, V) int) iter.Seq[V] {
	return func(yield func(V) bool) {
		next, stop := iter.Pull(y)
		defer stop()
		v2, ok2 := next()
		for v1 := range x {
			for ok2 && f(v1, v2) > 0 {
				if !yield(v2) {
					return
				}
				v2, ok2 = next()
			}
			if !yield(v1) {
				return
			}
		}
		for ok2 {
			if !yield(v2) {
				return
			}
			v2, ok2 = next()
		}
	}
}

func MapStringers[S ~[]T, T fmt.Stringer](s S) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, e := range s {
			if !yield(e.String()) {
				return
			}
		}
	}
}
