package repo

import (
	"context"
	"fmt"
	"log/slog"
	"path"

	"github.com/bluesky-social/indigo/mst"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multicodec"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/internal/cbor/dagcbor"
)

const repoVersion int64 = 3

var DefaultPrefix = cid.NewPrefixV1(
	uint64(multicodec.DagCbor),
	defaultMultihash,
)

func NewCID(value any) (c cid.Cid, err error) {
	b, err := dagcbor.Marshal(value)
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
	signer  Signer
	log     *slog.Logger
}

type Signer func(ctx context.Context, did string, data []byte) ([]byte, error)

func New(did string, storage Blockstore, signer Signer) *Repo {
	repo := newRepo(storage, signer, mst.NewEmptyMST(&ipldStore{bs: storage}))
	repo.dirty = true
	repo.commit = SignedCommit{
		Version: repoVersion,
		DID:     did,
	}
	return repo
}

func Load(ctx context.Context, storage Blockstore, root cid.Cid, signer Signer) (*Repo, error) {
	repo := newRepo(storage, signer, mst.LoadMST(&ipldStore{bs: storage}, root))
	repo.root = root
	blk, err := storage.Get(ctx, root)
	if err != nil {
		return nil, err
	}
	err = dagcbor.Unmarshal(blk.RawData(), &repo.commit)
	if err != nil {
		return nil, err
	}
	if err = repo.maybeSetStorageRev(repo.commit.Rev); err != nil {
		return nil, err
	}
	return repo, nil
}

func newRepo(storage Blockstore, signer Signer, mst *mst.MerkleSearchTree) *Repo {
	return &Repo{
		storage: storage,
		signer:  signer,
		mst:     mst,
		log:     slog.Default().With("component", "repo.Repo"),
	}
}

func (r *Repo) SignedCommit() *SignedCommit {
	return &r.commit
}

func (r *Repo) getTree() *mst.MerkleSearchTree {
	if r.mst == nil {
		r.mst = mst.LoadMST(&ipldStore{bs: r.storage}, r.commit.Data)
	}
	return r.mst
}

func (r *Repo) GetRecordBytes(ctx context.Context, collection, rkey string) (cid.Cid, []byte, error) {
	t := r.getTree()
	key := path.Join(collection, rkey)
	fmt.Printf("mst.Get(%q)\n", key)
	c, err := t.Get(ctx, key)
	if err != nil {
		return cid.Undef, nil, errors.WithStack(err)
	}
	block, err := r.storage.Get(ctx, c)
	if err != nil {
		return cid.Undef, nil, err
	}
	return c, block.RawData(), nil
}

func (r *Repo) GetRecord(ctx context.Context, collection, rkey string, out any) error {
	_, raw, err := r.GetRecordBytes(ctx, collection, rkey)
	if err != nil {
		return err
	}
	return dagcbor.Unmarshal(raw, out)
}

func (r *Repo) DeleteRecord(ctx context.Context, collection, rkey string) error {
	t := r.getTree()
	mst, err := t.Delete(ctx, path.Join(collection, rkey))
	if err != nil {
		return err
	}
	r.dirty = true
	r.mst = mst
	return nil
}

func (r *Repo) PutRecord(ctx context.Context, collection, rkey string, record any) (cid.Cid, error) {
	t := r.getTree()
	cst := ipldStore{r.storage}
	r.dirty = true
	c, err := cst.Put(ctx, record)
	if err != nil {
		return cid.Undef, err
	}
	mst, err := t.Add(ctx, path.Join(collection, rkey), c, -1)
	if err != nil {
		return cid.Undef, err
	}
	r.mst = mst
	return c, nil
}

func (r *Repo) UpdateRecord(ctx context.Context, collection, rkey string, record any) (cid.Cid, error) {
	t := r.getTree()
	cst := ipldStore{r.storage}
	r.dirty = true
	c, err := cst.Put(ctx, record)
	if err != nil {
		return cid.Undef, err
	}
	mst, err := t.Update(ctx, path.Join(collection, rkey), c)
	if err != nil {
		return cid.Undef, err
	}
	r.mst = mst
	return c, nil
}

type WriteResult struct {
	CID cid.Cid
	Op  *RecordWriteOp
}

func (r *Repo) ApplyWrites(ctx context.Context, writes []RecordWriteOp) ([]WriteResult, error) {
	res := make([]WriteResult, 0)
	t := r.getTree()
	cst := ipldStore{r.storage}
	r.dirty = true
	for i, write := range writes {
		key := path.Join(write.Collection, write.RecordKey)
		switch write.Action {
		case WriteOpActionCreate:
			c, err := cst.Put(ctx, write.Record)
			if err != nil {
				return nil, err
			}
			res = append(res, WriteResult{CID: c, Op: &writes[i]})
			mst, err := t.Add(ctx, key, c, -1)
			if err != nil {
				return nil, err
			}
			r.mst = mst
		case WriteOpActionUpdate:
			c, err := cst.Put(ctx, write.Record)
			if err != nil {
				return nil, err
			}
			res = append(res, WriteResult{CID: c, Op: &writes[i]})
			mst, err := t.Update(ctx, key, c)
			if err != nil {
				return nil, err
			}
			r.mst = mst
		case WriteOpActionDelete:
			mst, err := t.Delete(ctx, key)
			if err != nil {
				return nil, err
			}
			r.mst = mst
			res = append(res, WriteResult{CID: cid.Undef, Op: &writes[i]})
		default:
			return nil, errors.Errorf("unknown write operation %q", write.Action)
		}
	}
	return res, nil
}

func (r *Repo) Commit(ctx context.Context) (cid.Cid, string, error) {
	t := r.getTree()
	rcid, err := t.GetPointer(ctx)
	if err != nil {
		return cid.Undef, "", err
	}
	ucommit := UnsignedCommit{
		DID:     r.commit.DID,
		Version: repoVersion,
		Data:    rcid,
		Rev:     tid.Next().String(),
	}
	rawUnsignedCommit, err := dagcbor.Marshal(&ucommit)
	if err != nil {
		return cid.Undef, "", err
	}
	sig, err := r.signer(ctx, ucommit.DID, rawUnsignedCommit)
	if err != nil {
		return cid.Undef, "", err
	}
	commit := SignedCommit{
		Sig:     sig,
		DID:     ucommit.DID,
		Version: ucommit.Version,
		Prev:    ucommit.Prev,
		Data:    ucommit.Data,
		Rev:     ucommit.Rev,
	}
	if err = r.maybeSetStorageRev(commit.Rev); err != nil {
		return cid.Undef, "", err
	}
	store := ipldStore{bs: r.storage}
	c, err := store.Put(ctx, &commit)
	if err != nil {
		return cid.Undef, "", err
	}
	r.commit = commit
	r.dirty = false
	return c, commit.Rev, nil
}

func (r *Repo) DiffSince(ctx context.Context, oldrepo cid.Cid) ([]*mst.DiffOp, error) {
	var oldTree cid.Cid
	if oldrepo.Defined() {
		otherRepo, err := Load(ctx, r.storage, oldrepo, r.signer)
		if err != nil {
			return nil, err
		}
		oldmst := otherRepo.getTree()
		oldptr, err := oldmst.GetPointer(ctx)
		if err != nil {
			return nil, err
		}
		oldTree = oldptr
	}
	curmst := r.getTree()
	curptr, err := curmst.GetPointer(ctx)
	if err != nil {
		return nil, err
	}
	return mst.DiffTrees(
		ctx,
		&ipldBlockstore{r.storage},
		oldTree,
		curptr,
	)
}

func (r *Repo) FormatCommit(
	ctx context.Context,
	writes []RecordWriteOp,
) (*CommitData, error) {
	return FormatCommit(ctx, r, writes)
}

// lazy hack :(
func (r *Repo) maybeSetStorageRev(rev string) error {
	switch s := r.storage.(type) {
	case interface{ SetRev(string) }:
		s.SetRev(rev)
	case interface{ SetRev(string) error }:
		err := s.SetRev(rev)
		if err != nil {
			return err
		}
	}
	return nil
}

type ipldStore struct{ bs Blockstore }

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

type ipldBlockstore struct{ Blockstore }

func (ib *ipldBlockstore) DeleteBlock(ctx context.Context, c cid.Cid) error {
	if i, ok := ib.Blockstore.(interface {
		DeleteBlock(context.Context, cid.Cid) error
	}); ok {
		return i.DeleteBlock(ctx, c)
	}
	return errors.New("not implemented")
}

func (ib *ipldBlockstore) GetSize(ctx context.Context, c cid.Cid) (int, error) {
	if i, ok := ib.Blockstore.(interface {
		GetSize(context.Context, cid.Cid) (int, error)
	}); ok {
		return i.GetSize(ctx, c)
	}
	return 0, errors.New("not implemented")
}

func (ib *ipldBlockstore) Has(ctx context.Context, cid cid.Cid) (bool, error) {
	_, err := ib.Get(ctx, cid)
	if err == nil {
		return true, nil
	}
	return false, err
}

func (ib *ipldBlockstore) PutMany(ctx context.Context, blks []blocks.Block) error {
	if i, ok := ib.Blockstore.(interface {
		PutMany(context.Context, []blocks.Block) error
	}); ok {
		return i.PutMany(ctx, blks)
	}
	for _, blk := range blks {
		err := ib.Put(ctx, blk)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ib *ipldBlockstore) AllKeysChan(ctx context.Context) (<-chan cid.Cid, error) {
	if i, ok := ib.Blockstore.(interface {
		AllKeysChan(ctx context.Context) (<-chan cid.Cid, error)
	}); ok {
		return i.AllKeysChan(ctx)
	}
	return nil, errors.New("not implemented")
}

func (ib *ipldBlockstore) HashOnRead(enabled bool) {
	if i, ok := ib.Blockstore.(interface{ HashOnRead(bool) }); ok {
		i.HashOnRead(enabled)
	}
}

type dataAdd struct {
	key string
	cid cid.Cid
}

type dataUpdate struct {
	key  string
	cid  cid.Cid
	prev cid.Cid
}

type dataDelete struct {
	key string
	cid cid.Cid
}

type dataDiff struct {
	adds        map[string]dataAdd
	updates     map[string]dataUpdate
	deletes     map[string]dataDelete
	newblocks   *BlockMap
	newleafCids *cid.Set
	removedCids *cid.Set
}

func diffOf(ctx context.Context, bs Blockstore, from, to cid.Cid) (*dataDiff, error) {
	diff := dataDiff{
		adds:        make(map[string]dataAdd),
		updates:     make(map[string]dataUpdate),
		deletes:     make(map[string]dataDelete),
		newblocks:   NewBlockMap(),
		newleafCids: cid.NewSet(),
		removedCids: cid.NewSet(),
	}
	ops, err := mst.DiffTrees(ctx, &ipldBlockstore{bs}, from, to)
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		fmt.Println(op)
	}
	return &diff, nil
}

func diffFrom(ctx context.Context, store Blockstore, ops []*mst.DiffOp) (*dataDiff, error) {
	diff := dataDiff{
		adds:        make(map[string]dataAdd),
		updates:     make(map[string]dataUpdate),
		deletes:     make(map[string]dataDelete),
		newblocks:   NewBlockMap(),
		newleafCids: cid.NewSet(),
		removedCids: cid.NewSet(),
	}
	for _, op := range ops {
		switch op.Op {
		case "add":
			blk, err := store.Get(ctx, op.NewCid)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			diff.newblocks.Set(op.NewCid, blk.RawData())
		case "del":
			diff.removedCids.Add(op.NewCid)
		case "mut":
			diff.removedCids.Add(op.OldCid)
		default:
			return nil, errors.Errorf("unknown mst diff operation %q", op.Op)
		}
	}
	return &diff, nil
}

// func (dd *dataDiff) nodeAdd(mst.DiffOp)
// func (dd *dataDiff) nodeDelete(mst.DiffOp)

func (dd *dataDiff) leafAdd(key string, cid cid.Cid) {
	dd.adds[key] = dataAdd{key: key, cid: cid}
	if dd.removedCids.Has(cid) {
		dd.removedCids.Remove(cid)
	} else {
		dd.newleafCids.Add(cid)
	}
}

func (dd *dataDiff) leafUpdate(key string, prev, cid cid.Cid) {
	if prev.Equals(cid) {
		return
	}
	dd.updates[key] = dataUpdate{key: key, cid: cid, prev: prev}
	dd.removedCids.Add(prev)
	dd.newleafCids.Add(cid)
}

func (dd *dataDiff) leafDelete(key string, c cid.Cid) {
	dd.deletes[key] = dataDelete{key: key, cid: c}
	if dd.newleafCids.Has(c) {
		dd.newleafCids.Remove(c)
	} else {
		dd.removedCids.Add(c)
	}
}

func (dd *dataDiff) treeAdd(c cid.Cid, b []byte) {
	if dd.removedCids.Has(c) {
		dd.removedCids.Remove(c)
	} else {
		dd.newblocks.Set(c, b)
	}
}

func (dd *dataDiff) treeDelete(c cid.Cid) {
	if dd.newblocks.Has(c) {
		dd.newblocks.Delete(c)
	} else {
		dd.removedCids.Add(c)
	}
}
