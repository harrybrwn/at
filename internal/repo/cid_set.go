package repo

import (
	"iter"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/pkg/errors"
)

func IterCIDSet(set *cid.Set) iter.Seq[cid.Cid] {
	return func(yield func(cid.Cid) bool) {
		_ = set.ForEach(func(c cid.Cid) error {
			if !yield(c) {
				return errors.New("stop loop")
			}
			return nil
		})
	}
}

type CIDSet struct {
	set map[string]struct{}
	mu  sync.Mutex
}

// NewCIDSet initializes a new CIDSet, optionally with an array of CIDs.
func NewCIDSet(arr []string) *CIDSet {
	set := make(map[string]struct{})
	for _, cid := range arr {
		set[cid] = struct{}{}
	}
	return &CIDSet{set: set}
}

// Add adds a CID to the set.
func (cs *CIDSet) Add(cid string) *CIDSet {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.set[cid] = struct{}{}
	return cs
}

// AddSet merges another CIDSet into this set.
func (cs *CIDSet) AddSet(toMerge *CIDSet) *CIDSet {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for cid := range toMerge.set {
		cs.set[cid] = struct{}{}
	}
	return cs
}

// SubtractSet removes all elements of another CIDSet from this set.
func (cs *CIDSet) SubtractSet(toSubtract *CIDSet) *CIDSet {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for cid := range toSubtract.set {
		delete(cs.set, cid)
	}
	return cs
}

// Delete removes a CID from the set.
func (cs *CIDSet) Delete(cid string) *CIDSet {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.set, cid)
	return cs
}

// Has checks if a CID exists in the set.
func (cs *CIDSet) Has(cid string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	_, exists := cs.set[cid]
	return exists
}

// Size returns the number of elements in the set.
func (cs *CIDSet) Size() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.set)
}

// Clear removes all elements from the set.
func (cs *CIDSet) Clear() *CIDSet {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.set = make(map[string]struct{})
	return cs
}

// ToList returns the CIDs in the set as a slice.
func (cs *CIDSet) ToList() []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cids := make([]string, 0, len(cs.set))
	for cid := range cs.set {
		cids = append(cids, cid)
	}
	return cids
}

func (cs *CIDSet) Iter() iter.Seq[string] {
	return func(yield func(string) bool) {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		for k := range cs.set {
			if !yield(k) {
				return
			}
		}
	}
}
