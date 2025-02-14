package repo

import (
	"context"
	"path"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/mst"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multihash"
	"github.com/pkg/errors"
)

const defaultMultihash = multihash.SHA2_256

func FormatInitCommit(
	ctx context.Context,
	storage blockstore.Blockstore,
	did string,
	key crypto.PrivateKey,
	initialWrites []RecordWriteOp,
) (*CommitData, error) {
	newBlocks := NewBlockMap()
	cbs := cbornode.NewCborStore(storage)
	cbs.DefaultMultihash = defaultMultihash
	tree := mst.NewEmptyMST(cbs)
	for _, w := range initialWrites {
		cid, err := newBlocks.Add(w.Record)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		tree, err = tree.Add(
			ctx,
			path.Join(w.Collection, w.RecordKey),
			cid, -1,
		)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	dataCid, err := tree.GetPointer(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// Here from and to are the same CID. I'm not sure if that's correct because
	// the typescript implementation calls `DataDiff.of(from, null)`.
	diffs, err := mst.DiffTrees(ctx, storage, dataCid, dataCid)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	removedCids := cid.NewSet()

	for _, diff := range diffs {
		blk, err := storage.Get(ctx, diff.NewCid)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cid := diff.NewCid
		switch diff.Op {
		case "add":
			newBlocks.Set(cid, blk.RawData())
			if removedCids.Has(cid) {
				removedCids.Remove(cid)
			}
		case "del":
			removedCids.Add(cid)
		case "mut":
			removedCids.Add(diff.OldCid)
		default:
			return nil, errors.Errorf("unknown mst diff operation %q", diff.Op)
		}
	}
	rev := tid.Next().String()
	commitCid, commit, err := SignCommit(&UnsignedCommit{
		DID:     did,
		Version: repoVersion,
		Rev:     rev,
		Prev:    nil,
		Data:    dataCid,
	}, key)
	if err != nil {
		return nil, err
	}
	newBlocks.Set(commitCid, commit.Bytes())
	cd := CommitData{
		CID:            commitCid,
		Rev:            rev,
		Since:          "",
		Prev:           cid.Undef,
		NewBlocks:      newBlocks,
		RelevantBlocks: newBlocks,
		RemovedCIDs:    removedCids,
		SignedCommit:   commit,
	}
	return &cd, nil
}

func FormatCommit(ctx context.Context, r *Repo, writes []RecordWriteOp) (*CommitData, error) {
	var (
		err    error
		leaves = NewBlockMap()
		data   = r.getTree()
	)

	for _, write := range writes {
		key := path.Join(write.Collection, write.RecordKey)
		switch write.Action {
		case WriteOpActionCreate:
			cid, err := leaves.Add(write.Record)
			if err != nil {
				return nil, err
			}
			data, err = data.Add(ctx, key, cid, -1)
			if err != nil {
				return nil, err
			}
		case WriteOpActionUpdate:
			cid, err := leaves.Add(write.Record)
			if err != nil {
				return nil, err
			}
			data, err = data.Update(ctx, key, cid)
			if err != nil {
				return nil, err
			}
		case WriteOpActionDelete:
			data, err = data.Delete(ctx, key)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.Errorf("unknown write operation %q", write.Action)
		}
	}

	dataCid, err := data.GetPointer(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// diffOps, err := r.DiffSince(ctx, r.root)
	diffOps, err := mst.DiffTrees(
		ctx,
		&ipldBlockstore{r.storage},
		r.root,
		dataCid,
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	relavanteBlocks := NewBlockMap()
	newBlocks := NewBlockMap()
	removedCids := cid.NewSet()
	for _, op := range diffOps {
		r.log.Debug("handling diff operation",
			"op", op.Op, "depth", op.Depth,
			"new_cid", op.NewCid, "old_cid", op.OldCid,
			"rpath", op.Rpath)
		switch op.Op {
		case "add":
			block, ok := leaves.Get(op.NewCid)
			if ok {
				newBlocks.Set(op.NewCid, block)
			}
		case "mut":
			// TODO
		case "del":
			// TODO
		default:
			return nil, errors.Errorf("unknown mst diff operation %q", op.Op)
		}
	}

	for _, w := range writes {
		// TODO figure out how mstAddBlocksForPath should be implemented
		err = mstAddBlocksForPath(data, path.Join(w.Collection, w.RecordKey), relavanteBlocks)
		if err != nil {
			return nil, err
		}
	}

	// TODO add leaf nodes from the diff to newBlocks and relavantBlocks

	rev := tid.Next().String()
	commitCid, commit, err := SignCommitWithSigner(&UnsignedCommit{
		Version: 3,
		DID:     r.commit.DID,
		Rev:     rev,
		Data:    dataCid,
		Prev:    nil,
	}, r.signer)
	if err != nil {
		return nil, err
	}
	commitData := CommitData{
		CID:            commitCid,
		Rev:            rev,
		Prev:           cid.Undef,
		NewBlocks:      newBlocks,
		RelevantBlocks: relavanteBlocks,
		RemovedCIDs:    removedCids,
		SignedCommit:   commit,
	}
	if commit.Prev != nil {
		commitData.Prev = *commit.Prev
	}
	if !commitCid.Equals(r.root) {
		commitData.NewBlocks.Set(commitCid, commit.Bytes())
		commitData.RelevantBlocks.Set(commitCid, commit.Bytes())
		commitData.RemovedCIDs.Add(r.root)
	}
	return &commitData, nil
}

func mstAddBlocksForPath(mst *mst.MerkleSearchTree, key string, blocks *BlockMap) error {
	// TODO see https://github.com/bluesky-social/atproto/blob/main/packages/repo/src/mst/mst.ts
	return nil
}
