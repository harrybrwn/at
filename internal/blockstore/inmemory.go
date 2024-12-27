package blockstore

import (
	"context"
	"sync"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
	"github.com/pkg/errors"
)

type memory struct {
	mu     sync.RWMutex
	m      map[cid.Cid]block.Block
	prefix cid.Prefix
}

func InMemory() *memory {
	prefix := cid.NewPrefixV1(
		uint64(multicodec.DagCbor),
		multihash.SHA2_256,
	)
	return InMemoryWithCIDPrefix(prefix)
}

func InMemoryWithCIDPrefix(prefix cid.Prefix) *memory {
	return &memory{
		m:      make(map[cid.Cid]block.Block),
		prefix: prefix,
	}
}

func (m *memory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m = make(map[cid.Cid]block.Block)
}

func (m *memory) Get(_ context.Context, c cid.Cid) (block.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	blk, ok := m.m[c]
	if !ok {
		return nil, errors.Errorf("block %q not found", c)
	}
	return blk, nil
}

func (m *memory) Put(_ context.Context, blk block.Block) error {
	cid, err := m.prefix.Sum(blk.RawData())
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[cid] = blk
	return nil
}

func (m *memory) DeleteBlock(_ context.Context, c cid.Cid) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, c)
	return nil
}

func (m *memory) Has(_ context.Context, c cid.Cid) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.m[c]
	return ok, nil
}

func (m *memory) PutMany(_ context.Context, blocks []block.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, blk := range blocks {
		cid, err := m.prefix.Sum(blk.RawData())
		if err != nil {
			return err
		}
		m.m[cid] = blk
	}
	return nil
}

func (m *memory) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	ch := make(chan cid.Cid)
	go func() {
		defer close(ch)
		m.mu.RLock()
		defer m.mu.RUnlock()
		for cid := range m.m {
			ch <- cid
		}
	}()
	return ch, nil
}

func (m *memory) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	block, err := m.Get(ctx, c)
	if err != nil {
		return 0, err
	}
	return len(block.RawData()), nil
}

func (m *memory) HashOnRead(enabled bool) {}
