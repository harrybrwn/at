package atp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/whyrusleeping/go-did"
)

type PLC struct {
	*Resolver
	Host string
}

type CreateOp struct {
	Type        string  `json:"type" cborgen:"type" cbor:"type"`
	SigningKey  string  `json:"signingKey" cborgen:"signingKey" cbor:"signingKey"`
	RecoveryKey string  `json:"recoveryKey" cborgen:"recoveryKey" cbor:"recoveryKey"`
	Handle      string  `json:"handle" cborgen:"handle" cbor:"handle"`
	Service     string  `json:"service" cborgen:"service" cbor:"service"`
	Prev        *string `json:"prev" cborgen:"prev" cbor:"prev"`
	Sig         string  `json:"sig" cborgen:"sig,omitempty" cbor:"sig,omitempty"`
}

func (p *PLC) CreateDID(ctx context.Context, signingkey *did.PrivKey, recovery, handle, service string) (string, error) {
	op := CreateOp{
		Type:        "create",
		SigningKey:  signingkey.Public().DID(),
		RecoveryKey: recovery,
		Handle:      handle,
		Service:     service,
	}
	var buf bytes.Buffer
	err := cbor.MarshalToBuffer(&op, &buf)
	if err != nil {
		return "", err
	}
	sig, err := signingkey.Sign(buf.Bytes())
	if err != nil {
		return "", err
	}
	op.Sig = base64.RawURLEncoding.EncodeToString(sig)
	opdid, err := didForCreateOp(&op)
	if err != nil {
		return "", err
	}
	req, err := p.opRequest(opdid, &op)
	if err != nil {
		return "", err
	}
	_ = req
	return createFakeDID(ctx)
}

func (plc *PLC) UpdateUserHandle(ctx context.Context, didstr, nhandle string) error {
	panic("not implemented")
}

func (p *PLC) opRequest(did string, op *CreateOp) (*http.Request, error) {
	req := http.Request{
		Method: "POST",
		Host:   p.Host,
		URL: &url.URL{
			Host: p.Host,
			Path: filepath.Join("/", did),
		},
	}
	body, err := json.Marshal(op)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(bytes.NewBuffer(body))
	return &req, nil
}

type FakePLC struct{ *Resolver }

func (p *FakePLC) CreateDID(ctx context.Context, sigkey *did.PrivKey, recovery, handle, service string) (string, error) {
	return createFakeDID(ctx)
}

func (plc *FakePLC) UpdateUserHandle(ctx context.Context, didstr, nhandle string) error {
	return nil
}

func createFakeDID(ctx context.Context) (string, error) {
	slog.WarnContext(ctx, "generating fake did")
	var buf [8]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		return "", err
	}
	return "did:plc:" + hex.EncodeToString(buf[:]), nil
}

func didForCreateOp(op *CreateOp) (string, error) {
	buf := new(bytes.Buffer)
	if err := cbor.MarshalToBuffer(&op, buf); err != nil {
		return "", err
	}

	h := sha256.Sum256(buf.Bytes())
	enchash := base32.StdEncoding.EncodeToString(h[:])
	enchash = strings.ToLower(enchash)
	return "did:plc:" + enchash[:24], nil
}
