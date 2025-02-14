package repo

import (
	"bytes"
	"context"
	"fmt"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/fxamacker/cbor/v2"
	"github.com/ipfs/go-cid"
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
	RemovedCIDs  *cid.Set      `json:"removedCids"`
	SignedCommit *SignedCommit `json:"-"`
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
	GetBlobs() []PreparedBlobRef
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

func (pc *PreparedCreate) GetAction() WriteOpAction    { return WriteOpActionCreate }
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

func (pu *PreparedUpdate) GetAction() WriteOpAction    { return WriteOpActionUpdate }
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

func (pd *PreparedDelete) GetAction() WriteOpAction    { return WriteOpActionUpdate }
func (pd *PreparedDelete) GetURI() syntax.ATURI        { return pd.URI }
func (pd *PreparedDelete) GetCID() cid.Cid             { return cid.Cid{} }
func (pd *PreparedDelete) GetSwapCID() *cid.Cid        { return pd.SwapCID }
func (pd *PreparedDelete) GetRecord() any              { return nil }
func (pd *PreparedDelete) GetBlobs() []PreparedBlobRef { return nil }

func PrepWrite[T WriteOp](v T) PreparedWrite {
	var p PreparedWrite
	action := v.GetAction()
	switch action {
	case WriteOpActionCreate:
		p.PreparedCreate = &PreparedCreate{
			Action:  action,
			URI:     v.GetURI(),
			CID:     v.GetCID(),
			SwapCID: v.GetSwapCID(),
			Record:  v.GetRecord(),
			Blobs:   v.GetBlobs(),
		}
	case WriteOpActionUpdate:
		p.PreparedUpdate = &PreparedUpdate{
			Action:  action,
			URI:     v.GetURI(),
			CID:     v.GetCID(),
			SwapCID: v.GetSwapCID(),
			Record:  v.GetRecord(),
			Blobs:   v.GetBlobs(),
		}
	case WriteOpActionDelete:
		p.PreparedDelete = &PreparedDelete{
			Action:  action,
			URI:     v.GetURI(),
			SwapCID: v.GetSwapCID(),
		}
	default:
		panic(fmt.Errorf("invalid write operation action %q", action))
	}
	return p
}

func PrepareWrite(action WriteOpAction, uri string, record any, swap *cid.Cid, blobs []PreparedBlobRef) (PreparedWrite, error) {
	var w PreparedWrite
	aturi, err := syntax.ParseATURI(uri)
	if err != nil {
		return w, errors.WithStack(err)
	}
	switch action {
	case WriteOpActionCreate:
		w.PreparedCreate = &PreparedCreate{}
	case WriteOpActionUpdate:
		w.PreparedUpdate = &PreparedUpdate{}
	case WriteOpActionDelete:
		w.PreparedDelete = &PreparedDelete{
			Action:  action,
			URI:     aturi,
			SwapCID: swap,
		}
		return w, nil
	default:
		return PreparedWrite{}, errors.New("unknown write action")
	}
	cid, err := NewCID(record)
	if err != nil {
		return PreparedWrite{}, err
	}
	w.SetInfo(action, cid, aturi, record, swap, blobs)
	return w, nil
}

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

// DeriveCID will encode the record stored and set the internal CID if the write
// operation has a Record and CID field.
func (pw *PreparedWrite) DeriveCID() (cid.Cid, error) {
	var (
		record any
		ptr    *cid.Cid
	)
	switch {
	case pw.PreparedCreate != nil:
		if pw.PreparedCreate.CID.Defined() {
			return pw.PreparedCreate.CID, nil
		}
		record = pw.PreparedCreate.Record
		ptr = &pw.PreparedCreate.CID
	case pw.PreparedUpdate != nil:
		if pw.PreparedUpdate.CID.Defined() {
			return pw.PreparedCreate.CID, nil
		}
		record = pw.PreparedUpdate.Record
		ptr = &pw.PreparedUpdate.CID
	default:
		return cid.Undef, nil
	}
	cid, err := NewCID(record)
	if err != nil {
		return cid, err
	}
	*ptr = cid
	return cid, nil
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

func (pw *PreparedWrite) SetInfo(
	action WriteOpAction,
	cid cid.Cid,
	uri syntax.ATURI,
	record any,
	swap *cid.Cid,
	blobs []PreparedBlobRef,
) {
	switch {
	case pw.PreparedCreate != nil:
		pw.PreparedCreate.Action = action
		pw.PreparedCreate.CID = cid
		pw.PreparedCreate.URI = uri
		pw.PreparedCreate.Record = record
		pw.PreparedCreate.SwapCID = swap
		pw.PreparedCreate.Blobs = blobs
	case pw.PreparedUpdate != nil:
		pw.PreparedUpdate.Action = action
		pw.PreparedUpdate.CID = cid
		pw.PreparedUpdate.URI = uri
		pw.PreparedUpdate.Record = record
		pw.PreparedUpdate.SwapCID = swap
		pw.PreparedUpdate.Blobs = blobs
	case pw.PreparedDelete != nil:
		pw.PreparedDelete.Action = action
		pw.PreparedDelete.URI = uri
		pw.PreparedDelete.SwapCID = swap
	}
}

func (pw *PreparedWrite) SetAction(action WriteOpAction) {
	switch {
	case pw.PreparedCreate != nil:
		pw.PreparedCreate.Action = action
	case pw.PreparedUpdate != nil:
		pw.PreparedUpdate.Action = action
	case pw.PreparedDelete != nil:
		pw.PreparedDelete.Action = action
	}
}

func (pw *PreparedWrite) SetRecord(r any) {
	switch {
	case pw.PreparedCreate != nil:
		pw.PreparedCreate.Record = r
	case pw.PreparedUpdate != nil:
		pw.PreparedUpdate.Record = r
	}
}

func (pw *PreparedWrite) SetCID(c cid.Cid) {
	switch {
	case pw.PreparedCreate != nil:
		pw.PreparedCreate.CID = c
	case pw.PreparedUpdate != nil:
		pw.PreparedUpdate.CID = c
	}
}

func (pw *PreparedWrite) SetURI(uri syntax.ATURI) {
	switch {
	case pw.PreparedCreate != nil:
		pw.PreparedCreate.URI = uri
	case pw.PreparedUpdate != nil:
		pw.PreparedUpdate.URI = uri
	case pw.PreparedDelete != nil:
		pw.PreparedDelete.URI = uri
	}
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

type SignedCommit struct {
	DID     string   `cbor:"did" cborgen:"did"`
	Version int64    `cbor:"version" cborgen:"version"`
	Prev    *cid.Cid `cbor:"prev" cborgen:"prev"`
	Data    cid.Cid  `cbor:"data" cborgen:"data"`
	Sig     []byte   `cbor:"sig" cborgen:"sig"`
	Rev     string   `cbor:"rev,omitempty" cborgen:"rev,omitempty"`

	raw []byte `cbor:"-" cborgen:"-" json:"-"`
}

func (sc *SignedCommit) Bytes() []byte { return sc.raw }

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

func SignCommit(c *UnsignedCommit, key crypto.PrivateKey) (cid.Cid, *SignedCommit, error) {
	return SignCommitWithSigner(c, func(ctx context.Context, did string, data []byte) ([]byte, error) {
		if did != c.DID {
			return nil, errors.New("did mismatch while signing commit")
		}
		return key.HashAndSign(data)
	})
}

func SignCommitWithSigner(c *UnsignedCommit, signer Signer) (cid.Cid, *SignedCommit, error) {
	var buf bytes.Buffer
	err := dagcbor.Encode(&buf, c)
	if err != nil {
		return cid.Undef, nil, err
	}
	b := buf.Bytes()
	sig, err := signer(context.TODO(), c.DID, b)
	if err != nil {
		return cid.Undef, nil, errors.WithStack(err)
	}
	sc := SignedCommit{
		DID:     c.DID,
		Version: c.Version,
		Data:    c.Data,
		Prev:    c.Prev,
		Sig:     sig,
		Rev:     c.Rev,
	}
	sc.raw, err = dagcbor.Marshal(&sc)
	if err != nil {
		return cid.Undef, nil, err
	}
	commitCid, err := DefaultPrefix.Sum(sc.raw)
	if err != nil {
		return cid.Undef, nil, errors.WithStack(err)
	}
	return commitCid, &sc, nil
}
