package repo

import (
	"context"
	"io"

	block "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
)

type Storage interface {
	GetRoot(ctx context.Context) (cid.Cid, error)
	GetRootDetailed(ctx context.Context) (*RootInfo, error)
	GetBytes(ctx context.Context, cid cid.Cid) ([]byte, error)
	Has(ctx context.Context, cid cid.Cid) (bool, error)
	PutBlock(ctx context.Context, cid cid.Cid, block []byte, rev string) error
	PutMany(ctx context.Context, blocks *BlockMap, rev string) error
	DeleteMany(ctx context.Context, cids *cid.Set) error
	GetBlocks(ctx context.Context, cids []cid.Cid) (*BlockResult, error)
	ApplyCommit(ctx context.Context, commit *CommitData, isCreate bool) error
	UpdateRoot(ctx context.Context, cid cid.Cid, rev string, isCreate bool) error

	// ReadRecord(ctx context.Context, cid syntax.CID) (Record, error)
	// Get(ctx context.Context, cid cid.Cid, out any) error
	// Put(ctx context.Context, v any) (cid.Cid, error)
}

type RootInfo struct {
	CID cid.Cid
	Rev string
}

type BlockResult struct {
	Blocks  *BlockMap
	Missing []cid.Cid
}

type BlobStore interface {
	MakePermanent(ctx context.Context, key string, cid cid.Cid) error
	PutPermanent(ctx context.Context, cid cid.Cid, r io.Reader) error
	Quarantine(ctx context.Context, cid cid.Cid) error
	Unquarantine(ctx context.Context, cid cid.Cid) error
	GetBytes(ctx context.Context, cid cid.Cid) ([]byte, error)
	GetStream(ctx context.Context, cid cid.Cid) (io.ReadCloser, error)
	HasStored(ctx context.Context, cid cid.Cid) (bool, error)
	Delete(ctx context.Context, cid cid.Cid) error
	DeleteMany(ctx context.Context, cids []cid.Cid) error
	// PutTemp stores an [io.Reader] as a temporary file and returns the key to
	// it's location.
	PutTemp(ctx context.Context, r io.Reader) (string, error)
	HasTemp(ctx context.Context, key string) (bool, error)
}

type Blockstore interface {
	Get(context.Context, cid.Cid) (block.Block, error)
	Put(context.Context, block.Block) error
}
