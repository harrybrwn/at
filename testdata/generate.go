package testdata

// Warning 0, 1, and 2 should not be exposed to the public.

//go:generate tar -xzvf 3.tar.gz

func init() {
	panic("DO NOT IMPORT THIS PACKAGE!")
}
