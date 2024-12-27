package repo

import (
	"context"

	"github.com/bluesky-social/indigo/mst"
	"github.com/fxamacker/cbor/v2"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multicodec"

	"github.com/harrybrwn/at/internal/cbor/dagcbor"
)

var DefaultPrefix = cid.NewPrefixV1(
	uint64(multicodec.DagCbor),
	defaultMultihash,
)

func NewCID(value any) (c cid.Cid, err error) {
	b, err := cbor.Marshal(value)
	if err != nil {
		return c, err
	}
	return DefaultPrefix.Sum(b)
}

func newCid(b []byte) (cid.Cid, error) {
	return DefaultPrefix.Sum(b)
}

type Repo struct {
	storage Blockstore
	root    cid.Cid
	mst     *mst.MerkleSearchTree
	commit  SignedCommit
	dirty   bool
}

func New(did string, storage Blockstore) (*Repo, error) {
	cst := cbornode.NewCborStore(storage)
	repo := Repo{
		storage: storage,
		mst:     mst.NewEmptyMST(cst),
		dirty:   true,
		commit: SignedCommit{
			Version: 2,
			DID:     did,
		},
	}
	return &repo, nil
}

func LoadRepo(ctx context.Context, storage Blockstore, root cid.Cid) (*Repo, error) {
	blk, err := storage.Get(ctx, root)
	if err != nil {
		return nil, err
	}
	repo := Repo{
		storage: storage,
		root:    root,
		mst:     mst.LoadMST(&ipldStore{bs: storage}, root),
	}
	err = cbor.Unmarshal(blk.RawData(), &repo.commit)
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

func (r *Repo) getTree() *mst.MerkleSearchTree {
	if r.mst == nil {
		r.mst = mst.LoadMST(&ipldStore{bs: r.storage}, r.commit.Data)
	}
	return r.mst
}

type ipldStore struct {
	bs Blockstore
}

func (is *ipldStore) Get(ctx context.Context, c cid.Cid, out any) error {
	block, err := is.bs.Get(ctx, c)
	if err != nil {
		return err
	}
	return dagcbor.Unmarshal(block.RawData(), out)
}

func (is *ipldStore) Put(ctx context.Context, v any) (cid.Cid, error) {
	b, err := dagcbor.Marshal(v)
	if err != nil {
		return cid.Undef, err
	}
	c, err := DefaultPrefix.Sum(b)
	if err != nil {
		return cid.Undef, err
	}
	blk, err := blocks.NewBlockWithCid(b, c)
	if err != nil {
		return cid.Undef, nil
	}
	return c, is.bs.Put(ctx, blk)
}

var _ cbornode.IpldStore = (*ipldStore)(nil)
