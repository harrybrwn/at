package array

func Find[T any](arr []T, fn func(*T) bool) *T {
	return Searchble[T](arr).Find(fn)
}

func FindAll[T any](arr []T, fn func(*T) bool) []*T {
	return Searchble[T](arr).FindAll(fn)
}

type Searchble[T any] []T

func (sa Searchble[T]) Len() int { return len(sa) }

func (sa Searchble[T]) Find(fn func(*T) bool) *T {
	for i := range sa {
		if fn(&sa[i]) {
			return &sa[i]
		}
	}
	return nil
}

func (sa Searchble[T]) FindAll(fn func(*T) bool) []*T {
	res := make([]*T, 0)
	for i := range sa {
		if fn(&sa[i]) {
			res = append(res, &sa[i])
		}
	}
	return res
}

func (sa Searchble[T]) FinaAllAppend(results *[]*T, fn func(*T) bool) {
	for i := range sa {
		if fn(&sa[i]) {
			*results = append(*results, &sa[i])
		}
	}
}