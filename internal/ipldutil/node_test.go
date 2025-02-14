package ipldutil

import "testing"

// NOTE:
// go install golang.org/x/perf/cmd/benchstat@latest
//
// go test -run '^$' -bench='.*' | tee ipldutil_benchmark0.txt
// go test -run '^$' -bench='.*' | tee ipldutil_benchmark1.txt
// benchstat ipldutil_benchmark0.txt ipldutil_benchmark1.txt

func BenchmarkBuildNode(b *testing.B) {}
