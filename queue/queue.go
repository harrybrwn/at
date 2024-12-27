package queue

import "iter"

type QueueElem[T any] struct {
	prev  *QueueElem[T]
	value T
}

type Queue[T any] struct {
	end *QueueElem[T]
	len int
}

func (q *Queue[T]) Init() {}
func (q *Queue[T]) Reset() {
	q.end = nil
	q.len = 0
}
func (q *Queue[T]) PushBack(v T)  { q.Push(v) }
func (q *Queue[T]) PushFront(v T) { q.Push(v) }
func (q *Queue[T]) Empty() bool   { return q.end == nil }
func (q *Queue[T]) Len() int      { return q.len }

func (q *Queue[T]) Push(v T) {
	n := QueueElem[T]{value: v, prev: q.end}
	q.end = &n
	q.len++
}

func (q *Queue[T]) Pop() (T, bool) {
	e := q.end
	if e == nil {
		var v T
		return v, false
	}
	q.end = e.prev
	q.len--
	return e.value, true
}

func (q *Queue[T]) Iter() iter.Seq[T] {
	return func(yield func(T) bool) {
		for e := q.end; e != nil; e = e.prev {
			if !yield(e.value) {
				return
			}
		}
	}
}

type ListQueue[T any] struct{ list List[T] }

func (q *ListQueue[T]) Init() { q.list.Init() }
func (q *ListQueue[T]) Reset() {
	q.list.root = Element[T]{}
	q.list.Init()
}
func (q *ListQueue[T]) Push(v T)     { q.list.PushFront(v) }
func (q *ListQueue[T]) PushBack(v T) { q.list.PushBack(v) }
func (q *ListQueue[T]) Empty() bool  { return q.list.len == 0 }
func (q *ListQueue[T]) Len() int     { return q.list.len }
func (q *ListQueue[T]) Pop() (T, bool) {
	if q.list.len == 0 {
		var v T
		return v, false
	}
	return q.list.Remove(q.list.Front()), true
}
func (q *ListQueue[T]) PopBack() T           { return q.list.Remove(q.list.Back()) }
func (q *ListQueue[T]) Iter() iter.Seq[T]    { return q.list.Iter() }
func (q *ListQueue[T]) RevIter() iter.Seq[T] { return q.list.RevIter() }