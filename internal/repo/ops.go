package repo

import (
	"bytes"
	"context"
	"path"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/indigo/mst"
	"github.com/fxamacker/cbor/v2"
	"github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/multiformats/go-multihash"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/internal/cbor/dagcbor"
)

type CommitData struct {
	CID            cid.Cid   `json:"cid"`
	Rev            string    `json:"rev"`
	Since          string    `json:"since"`
	Prev           cid.Cid   `json:"prev"`
	NewBlocks      *BlockMap `json:"newBlocks"`
	RelevantBlocks *BlockMap `json:"relevantBlocks"`
	// RemovedCIDs    *CIDSet   `json:"removedCids"`
	RemovedCIDs *cid.Set `json:"removedCids"`
}

func BlocksToCarFile(root syntax.CID, blocks *BlockMap) ([]byte, error) {
	return nil, nil
}

type WriteOpAction string

const (
	WriteOpActionCreate WriteOpAction = "Create"
	WriteOpActionUpdate WriteOpAction = "Update"
	WriteOpActionDelete WriteOpAction = "Delete"
)

type ValidationStatus string

const (
	ValidationStatusValid   ValidationStatus = "valid"
	ValidationStatusUnknown ValidationStatus = "unknown"
)

type Record any

type BlobConstraint struct {
	Accept  []string `json:"accept,omitempty"`  // Optional field: array of strings
	MaxSize *int     `json:"maxSize,omitempty"` // Optional field: pointer to handle nullability
}

type PreparedBlobRef struct {
	CID         cid.Cid        `json:"cid"`
	MimeType    string         `json:"mimeType"`
	Constraints BlobConstraint `json:"constraints"`
}

type WriteOp interface {
	GetAction() WriteOpAction
	GetURI() syntax.ATURI
	GetCID() cid.Cid
	GetSwapCID() *cid.Cid
	GetRecord() any
}

type PreparedCreate struct {
	Action           WriteOpAction     `json:"action" cbor:"action"`
	URI              syntax.ATURI      `json:"uri" cbor:"uri"`
	CID              cid.Cid           `json:"cid" cbor:"cid"`
	SwapCID          *cid.Cid          `json:"swapCid,omitempty" cbor:"swapCid,omitempty"`
	Record           Record            `json:"record" cbor:"record"`
	Blobs            []PreparedBlobRef `json:"blobs" cbor:"blobs"`
	ValidationStatus ValidationStatus  `json:"validationStatus" cbor:"validationStatus"`
}

func (pc *PreparedCreate) GetAction() WriteOpAction    { return pc.Action }
func (pc *PreparedCreate) GetURI() syntax.ATURI        { return pc.URI }
func (pc *PreparedCreate) GetCID() cid.Cid             { return pc.CID }
func (pc *PreparedCreate) GetSwapCID() *cid.Cid        { return pc.SwapCID }
func (pc *PreparedCreate) GetRecord() any              { return pc.Record }
func (pc *PreparedCreate) GetBlobs() []PreparedBlobRef { return pc.Blobs }

type PreparedUpdate struct {
	Action           WriteOpAction     `json:"action" cbor:"action"`
	URI              syntax.ATURI      `json:"uri" cbor:"uri"`
	CID              cid.Cid           `json:"cid" cbor:"cid"`
	SwapCID          *cid.Cid          `json:"swapCid,omitempty" cbor:"swapCid,omitempty"`
	Record           Record            `json:"record" cbor:"record"`
	Blobs            []PreparedBlobRef `json:"blobs" cbor:"blobs"`
	ValidationStatus ValidationStatus  `json:"validationStatus" cbor:"validationStatus"`
}

func (pu *PreparedUpdate) GetAction() WriteOpAction    { return pu.Action }
func (pu *PreparedUpdate) GetURI() syntax.ATURI        { return pu.URI }
func (pu *PreparedUpdate) GetCID() cid.Cid             { return pu.CID }
func (pu *PreparedUpdate) GetSwapCID() *cid.Cid        { return pu.SwapCID }
func (pu *PreparedUpdate) GetRecord() any              { return pu.Record }
func (pu *PreparedUpdate) GetBlobs() []PreparedBlobRef { return pu.Blobs }

type PreparedDelete struct {
	Action  WriteOpAction `json:"action" cbor:"action"`
	URI     syntax.ATURI  `json:"uri" cbor:"uri"`
	SwapCID *cid.Cid      `json:"swapCid,omitempty" cbor:"swapCid,omitempty"`
}

func (pd *PreparedDelete) GetAction() WriteOpAction    { return pd.Action }
func (pd *PreparedDelete) GetURI() syntax.ATURI        { return pd.URI }
func (pd *PreparedDelete) GetCID() cid.Cid             { return cid.Cid{} }
func (pd *PreparedDelete) GetSwapCID() *cid.Cid        { return pd.SwapCID }
func (pd *PreparedDelete) GetRecord() any              { return nil }
func (pd *PreparedDelete) GetBlobs() []PreparedBlobRef { return nil }

type PreparedWrite struct {
	*PreparedCreate
	*PreparedUpdate
	*PreparedDelete
}

func (pw *PreparedWrite) MarshalCBOR() ([]byte, error) {
	switch {
	case pw.PreparedCreate != nil:
		return cbor.Marshal(pw.PreparedCreate)
	case pw.PreparedUpdate != nil:
		return cbor.Marshal(pw.PreparedUpdate)
	case pw.PreparedDelete != nil:
		return cbor.Marshal(pw.PreparedDelete)
	}
	return cbor.Marshal(nil)
}

func (pw *PreparedWrite) UnmarshalCBOR(b []byte) error {
	var sniffer struct {
		Action WriteOpAction `cbor:"action"`
	}
	err := cbor.Unmarshal(b, &sniffer)
	if err != nil {
		return err
	}
	switch sniffer.Action {
	case WriteOpActionCreate:
		pw.PreparedCreate = new(PreparedCreate)
		return cbor.Unmarshal(b, pw.PreparedCreate)
	case WriteOpActionUpdate:
		pw.PreparedUpdate = new(PreparedUpdate)
		return cbor.Unmarshal(b, pw.PreparedUpdate)
	case WriteOpActionDelete:
		pw.PreparedDelete = new(PreparedDelete)
		return cbor.Unmarshal(b, pw.PreparedDelete)
	}
	return errors.Errorf("unknown write action %q", sniffer.Action)
}

func (pw *PreparedWrite) GetAction() WriteOpAction {
	switch {
	case pw.PreparedCreate != nil:
		return pw.PreparedCreate.Action
	case pw.PreparedUpdate != nil:
		return pw.PreparedUpdate.Action
	case pw.PreparedDelete != nil:
		return pw.PreparedDelete.Action
	}
	return ""
}

func (pw *PreparedWrite) GetCID() cid.Cid {
	switch {
	case pw.PreparedCreate != nil:
		return pw.PreparedCreate.CID
	case pw.PreparedUpdate != nil:
		return pw.PreparedUpdate.CID
	}
	return cid.Undef
}

func (pw *PreparedWrite) GetBlobs() []PreparedBlobRef {
	switch {
	case pw.PreparedCreate != nil:
		return pw.PreparedCreate.Blobs
	case pw.PreparedUpdate != nil:
		return pw.PreparedUpdate.Blobs
	}
	return nil
}

func (pw *PreparedWrite) GetURI() syntax.ATURI {
	switch {
	case pw.PreparedCreate != nil:
		return pw.PreparedCreate.URI
	case pw.PreparedUpdate != nil:
		return pw.PreparedUpdate.URI
	case pw.PreparedDelete != nil:
		return pw.PreparedDelete.URI
	}
	return ""
}

func (pw *PreparedWrite) GetSwapCID() *cid.Cid {
	switch {
	case pw.PreparedCreate != nil:
		return pw.PreparedCreate.SwapCID
	case pw.PreparedUpdate != nil:
		return pw.PreparedUpdate.SwapCID
	case pw.PreparedDelete != nil:
		return pw.PreparedDelete.SwapCID
	}
	return nil
}

func (pw *PreparedWrite) GetRecord() any {
	switch {
	case pw.PreparedCreate != nil:
		return pw.PreparedCreate.Record
	case pw.PreparedUpdate != nil:
		return pw.PreparedUpdate.Record
	}
	return nil
}

type EventType string

const (
	EventCommit    EventType = "commit"
	EventIdentity  EventType = "identity"
	EventAccount   EventType = "account"
	EventHandle    EventType = "handle"
	EventMigrate   EventType = "migrate"
	EventTombstone EventType = "tombstone"
	EventInfo      EventType = "info"
)

var (
	ErrBadCommitSwap = errors.New("bad commit swap")
	ErrBadRecordSwap = errors.New("bad record swap")
)

type RecordWriteOp struct {
	Action     WriteOpAction `cbor:"action" cborgen:"action"`
	Collection string        `cbor:"collection" cborgen:"collection"`
	RecordKey  string        `cbor:"rkey" cborgen:"rkey"`
	Record     Record        `cbor:"record,omitempty" cborgen:"record,omitempty"`
}

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
		cid, err := newBlocks.Add(w)
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
		}
	}
	rev := tid.Next()
	commit, err := signCommit(&UnsignedCommit{
		DID:     did,
		Version: 3,
		Rev:     rev.String(),
		Prev:    nil,
		Data:    dataCid,
	}, key)
	if err != nil {
		return nil, err
	}
	commitCid, err := newBlocks.Add(commit)
	if err != nil {
		return nil, err
	}
	cd := CommitData{
		CID:            commitCid,
		Rev:            rev.String(),
		Since:          "",
		Prev:           cid.Undef,
		NewBlocks:      newBlocks,
		RelevantBlocks: newBlocks,
		RemovedCIDs:    removedCids,
	}
	if commit.Prev != nil {
		cd.Prev = *commit.Prev
	}
	return &cd, nil
}

type CID []byte

type SignedCommit struct {
	DID     string   `cbor:"did" cborgen:"did"`
	Version int64    `cbor:"version" cborgen:"version"`
	Prev    *cid.Cid `cbor:"prev" cborgen:"prev"`
	Data    cid.Cid  `cbor:"data" cborgen:"data"`
	Sig     []byte   `cbor:"sig" cborgen:"sig"`
	Rev     string   `cbor:"rev,omitempty" cborgen:"rev,omitempty"`
}

type UnsignedCommit struct {
	DID     string   `cbor:"did" cborgen:"did"`
	Version int64    `cbor:"version" cborgen:"version"`
	Prev    *cid.Cid `cbor:"prev" cborgen:"prev"`
	Data    cid.Cid  `cbor:"data" cborgen:"data"`
	Rev     string   `cbor:"rev,omitempty" cborgen:"rev,omitempty"`
}

var tid syntax.TIDClock

func NextTID() syntax.TID {
	return tid.Next()
}

func init() {
	dagcbor.Register(UnsignedCommit{})
}

func signCommit(c *UnsignedCommit, key crypto.PrivateKey) (*SignedCommit, error) {
	var buf bytes.Buffer
	err := dagcbor.Encode(&buf, c)
	// b, err := dagcbor.Marshal(c)
	if err != nil {
		return nil, err
	}
	b := buf.Bytes()
	sig, err := key.HashAndSign(b)
	if err != nil {
		return nil, err
	}
	sc := SignedCommit{
		DID:     c.DID,
		Version: c.Version,
		Data:    c.Data,
		Prev:    c.Prev,
		Sig:     sig,
		Rev:     c.Rev,
	}
	return &sc, nil
}
