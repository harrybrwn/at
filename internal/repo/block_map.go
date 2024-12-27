package repo

import (
	"bytes"
	"iter"
	"sync"

	"github.com/ipfs/go-cid"

	"github.com/harrybrwn/at/internal/cbor/dagcbor"
)

// Entry represents a CID-bytes pair
type Entry struct {
	CID   cid.Cid
	Bytes []byte
}

// BlockMap represents a thread-safe map of CID to byte content
type BlockMap struct {
	mu     sync.RWMutex
	blocks map[cid.Cid][]byte
}

// NewBlockMap creates a new BlockMap instance
func NewBlockMap() *BlockMap {
	return &BlockMap{
		blocks: make(map[cid.Cid][]byte),
	}
}

func BlockMapFromMap(m map[cid.Cid][]byte) *BlockMap {
	return &BlockMap{blocks: m}
}

func (bm *BlockMap) Add(value any) (c cid.Cid, err error) {
	b, err := dagcbor.Marshal(value)
	if err != nil {
		return cid.Undef, err
	}
	c, err = DefaultPrefix.Sum(b)
	if err != nil {
		return cid.Undef, err
	}
	bm.Set(c, b)
	return c, nil
}

// Set adds or updates a block in the map
func (bm *BlockMap) Set(cid cid.Cid, bytes []byte) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.blocks[cid] = bytes
}

// Get retrieves a block from the map
func (bm *BlockMap) Get(cid cid.Cid) ([]byte, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	block, ok := bm.blocks[cid]
	return block, ok
}

// Delete removes a block from the map
func (bm *BlockMap) Delete(cid cid.Cid) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	delete(bm.blocks, cid)
}

// GetMany retrieves multiple blocks and reports missing ones
func (bm *BlockMap) GetMany(cids []cid.Cid) (blocks *BlockMap, missing []cid.Cid) {
	blocks = NewBlockMap()
	missing = make([]cid.Cid, 0)

	bm.mu.RLock()
	defer bm.mu.RUnlock()

	for _, cid := range cids {
		if got, exists := bm.blocks[cid]; exists {
			blocks.Set(cid, got)
		} else {
			missing = append(missing, cid)
		}
	}
	return blocks, missing
}

// Has checks if a CID exists in the map
func (bm *BlockMap) Has(cid cid.Cid) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	_, exists := bm.blocks[cid]
	return exists
}

// Clear removes all entries from the map
func (bm *BlockMap) Clear() {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.blocks = make(map[cid.Cid][]byte)
}

// ForEach iterates over all entries in the map
func (bm *BlockMap) ForEach(cb func(bytes []byte, cid cid.Cid)) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	for cid, bytes := range bm.blocks {
		cb(bytes, cid)
	}
}

func (bm *BlockMap) Iter() iter.Seq2[cid.Cid, []byte] {
	return func(yield func(cid.Cid, []byte) bool) {
		bm.mu.RLock()
		defer bm.mu.RUnlock()
		for cid, bytes := range bm.blocks {
			if !yield(cid, bytes) {
				return
			}
		}
	}
}

// Entries returns all entries in the map
func (bm *BlockMap) Entries() []Entry {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	entries := make([]Entry, 0, len(bm.blocks))
	for cid, bytes := range bm.blocks {
		entries = append(entries, Entry{CID: cid, Bytes: bytes})
	}
	return entries
}

// CIDs returns all CIDs in the map
func (bm *BlockMap) CIDs() []cid.Cid {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	cids := make([]cid.Cid, 0, len(bm.blocks))
	for cid := range bm.blocks {
		cids = append(cids, cid)
	}
	return cids
}

// AddMap adds all entries from another BlockMap
func (bm *BlockMap) AddMap(other *BlockMap) {
	other.ForEach(func(bytes []byte, cid cid.Cid) {
		bm.Set(cid, bytes)
	})
}

// Size returns the number of entries in the map
func (bm *BlockMap) Size() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return len(bm.blocks)
}

// ByteSize returns the total size of all blocks in bytes
func (bm *BlockMap) ByteSize() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	size := 0
	for _, bytes := range bm.blocks {
		size += len(bytes)
	}
	return size
}

// Equals compares this BlockMap with another for equality
func (bm *BlockMap) Equals(other *BlockMap) bool {
	if bm.Size() != other.Size() {
		return false
	}
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	other.mu.RLock()
	defer other.mu.RUnlock()
	for cid, b := range bm.blocks {
		otherBytes := other.blocks[cid]
		if otherBytes == nil {
			return false
		}
		if !bytes.Equal(b, otherBytes) {
			return false
		}
	}
	return true
}
