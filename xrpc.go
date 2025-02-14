package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/pkg/errors"

	"github.com/harrybrwn/at/xrpc"
)

type XRPCClient struct {
	pds        string
	AdminToken *string
}

func (c *XRPCClient) url(path string, q url.Values) (*url.URL, error) {
	u, err := url.Parse(c.pds)
	if err != nil {
		return nil, err
	}
	u.Path = path
	if q != nil {
		u.RawQuery = q.Encode()
	}
	return u, nil
}

func (c *XRPCClient) do(ctx context.Context, tp xrpc.RequestType, ns string, q url.Values) (*http.Response, error) {
	u, err := c.url("/xrpc/"+ns, q)
	if err != nil {
		return nil, err
	}
	req := http.Request{
		Host:   u.Host,
		URL:    u,
		Header: make(http.Header),
	}
	switch tp {
	case xrpc.Query:
		req.Method = "GET"
	case xrpc.Procedure:
		req.Method = "POST"
	}
	if c.AdminToken != nil && (strings.HasPrefix(ns, "com.atproto.admin.") ||
		strings.HasPrefix(ns, "tools.ozone.") ||
		ns == "com.atproto.server.createInviteCode" ||
		ns == "com.atproto.server.createInviteCodes") {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("admin:"+*c.AdminToken)))
	}
	res, err := HttpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return res, err
}

func (c *XRPCClient) dojson(ctx context.Context, tp xrpc.RequestType, ns string, q url.Values, dst any) error {
	res, err := c.do(ctx, tp, ns, q)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		e := Error{Status: res.StatusCode}
		_ = json.NewDecoder(res.Body).Decode(&e)
		return &e
	}
	return json.NewDecoder(res.Body).Decode(dst)
}

type Repo struct {
	Handle          string               `json:"handle"`
	Did             string               `json:"did"`
	DidDoc          identity.DIDDocument `json:"didDoc"`
	Collections     []string             `json:"collections"`
	HandleIsCorrect bool                 `json:"handleIsCorrect"`
}

func (c *XRPCClient) Repo(ctx context.Context, repo syntax.DID) (*Repo, error) {
	var (
		err error
		r   Repo
	)
	q := make(url.Values)
	q.Set("repo", string(repo))
	err = c.dojson(ctx, xrpc.Query, "com.atproto.repo.describeRepo", q, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

type RecordList[T any] struct {
	Records []Record[T] `json:"records"`
	Cursor  string      `json:"cursor"`
}

type Record[T any] struct {
	URI   string `json:"uri"`
	Cid   string `json:"cid"`
	Value T      `json:"value"`
}

type LabelerRecord struct {
	Type     string `json:"$type"`
	Policies struct {
		LabelValues           []string `json:"labelValues"`
		LabelValueDefinitions []struct {
			Blurs   string `json:"blurs"`
			Locales []struct {
				Lang        string `json:"lang"`
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"locales"`
			Severity       string `json:"severity"`
			AdultOnly      bool   `json:"adultOnly"`
			Identifier     string `json:"identifier"`
			DefaultSetting string `json:"defaultSetting"`
		} `json:"labelValueDefinitions"`
	} `json:"policies"`
	CreatedAt time.Time `json:"createdAt"`
}

type ProfileRecord struct {
	Type        string            `json:"$type"`
	Description string            `json:"description"`
	DisplayName string            `json:"displayName"`
	Avatar      *ProfileAvatar    `json:"avatar"`
	Banner      *ProfileBanner    `json:"banner"`
	PinnedPost  *ProfilePinedPost `json:"pinnedPost"`
}

type Ref struct {
	Link string `json:"$link"`
}

type ProfileAvatar struct {
	Type     string `json:"$type"`
	Ref      Ref    `json:"ref"`
	MimeType string `json:"mimeType"`
	Size     int    `json:"size"`
}

type ProfileBanner struct {
	Type     string `json:"$type"`
	Ref      Ref    `json:"ref"`
	MimeType string `json:"mimeType"`
	Size     int    `json:"size"`
}

type ProfilePinedPost struct {
	CID string `json:"cid"`
	URI string `json:"uri"`
}

type RecordValue struct {
	Type      string    `json:"$type"`
	Subject   *Subject  `json:"subject"`
	CreatedAt time.Time `json:"createdAt"`
}

type Subject struct {
	Cid string `json:"cid"`
	URI string `json:"uri"`
	// DID is populated when the subject is a string not an object.
	DID string `json:"-"`
}

func (s *Subject) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if b[0] == '"' && b[len(b)-1] == '"' {
		s.DID = string(b[1 : len(b)-1])
		return nil
	} else {
		type subject Subject
		return json.Unmarshal(b, (*subject)(s))
	}
}

func (s *Subject) MarshalJSON() ([]byte, error) {
	if s.IsDID() {
		return []byte(fmt.Sprintf("%q", s.DID)), nil
	} else {
		type subject Subject
		return json.Marshal((*subject)(s))
	}
}

func (s *Subject) IsDID() bool { return len(s.DID) > 0 }

func (c *XRPCClient) ListRecords(ctx context.Context, repo syntax.DID, collection syntax.NSID, limit int, cursor string) (*RecordList[map[string]any], error) {
	var list RecordList[map[string]any]
	q := make(url.Values)
	q.Set("repo", string(repo))
	q.Set("collection", string(collection))
	q.Set("limit", strconv.FormatInt(int64(limit), 10))
	if len(cursor) > 0 {
		q.Set("cursor", cursor)
	}
	return &list, c.dojson(ctx, xrpc.Query, "com.atproto.repo.listRecords", q, &list)
}

func (c *XRPCClient) LabelerRecord(ctx context.Context, repo syntax.DID, collection syntax.NSID, rkey syntax.RecordKey) (*Record[LabelerRecord], error) {
	var r Record[LabelerRecord]
	err := c.record(ctx, &r, repo, collection, rkey)
	return &r, err
}

func (c *XRPCClient) ProfileRecord(ctx context.Context, repo syntax.DID, collection syntax.NSID, rkey syntax.RecordKey) (*Record[ProfileRecord], error) {
	var r Record[ProfileRecord]
	err := c.record(ctx, &r, repo, collection, rkey)
	return &r, err
}
func (c *XRPCClient) Record(ctx context.Context, repo syntax.DID, collection syntax.NSID, rkey syntax.RecordKey) (*Record[map[string]any], error) {
	var r Record[map[string]any]
	err := c.record(ctx, &r, repo, collection, rkey)
	return &r, err
}

func (c *XRPCClient) record(ctx context.Context, dst any, repo syntax.DID, collection syntax.NSID, rkey syntax.RecordKey) error {
	err := c.dojson(ctx, xrpc.Query, "com.atproto.repo.getRecord", url.Values{
		"repo":       []string{repo.String()},
		"collection": []string{collection.String()},
		"rkey":       []string{rkey.String()},
	}, dst)
	if err != nil {
		return err
	}
	return nil
}

type BlobRefs struct {
	CIDs   []syntax.CID `json:"cids"`
	Cursor string       `json:"cursor"`
}

func (c *XRPCClient) ListBlobs(ctx context.Context, did syntax.DID, limit int, cursor string) (b *BlobRefs, err error) {
	b = new(BlobRefs)
	q := make(url.Values)
	q.Set("did", did.String())
	if limit > 0 {
		q.Set("limit", strconv.FormatInt(int64(limit), 10))
	}
	if len(cursor) > 0 {
		q.Set("cursor", cursor)
	}
	return b, c.dojson(ctx, xrpc.Query, "com.atproto.sync.listBlobs", q, b)
}

func (c *XRPCClient) GetBlob(ctx context.Context, did syntax.DID, cid syntax.CID, w io.Writer) error {
	res, err := c.do(ctx, xrpc.Query, "com.atproto.sync.getBlob", url.Values{
		"did": []string{did.String()},
		"cid": []string{cid.String()},
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, err = io.Copy(w, res.Body)
	return errors.WithStack(err)
}

func blobURL(pds string, did syntax.DID, cid syntax.CID) string {
	u, err := url.Parse(pds)
	if err != nil {
		return ""
	}
	u.Path = "/xrpc/com.atproto.sync.getBlob"
	u.RawQuery = fmt.Sprintf("did=%s&cid=%s", did, cid)
	return u.String()
}

func PlcHistory(did syntax.DID) ([]HistoryItem, error) {
	u := fmt.Sprintf(
		"https://%s/%s/log/audit",
		getEnv("ATP_PLC_HOST", "plc.directory"),
		did,
	)
	res, err := HttpClient.Get(u)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		var e xrpc.ErrorResponse
		err = json.NewDecoder(res.Body).Decode(&e)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		return nil, errors.WithStack(&e)
	}
	items := make([]HistoryItem, 0)
	return items, errors.WithStack(json.NewDecoder(res.Body).Decode(&items))
}

func PlcHistoryLast(did syntax.DID) (*HistoryOperation, error) {
	u := fmt.Sprintf(
		"https://%s/%s/log/last",
		getEnv("ATP_PLC_HOST", "plc.directory"),
		did,
	)
	res, err := HttpClient.Get(u)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer res.Body.Close()
	var op HistoryOperation
	return &op, errors.WithStack(json.NewDecoder(res.Body).Decode(&op))
}

type HistoryItem struct {
	DID       string           `json:"did"`
	Operation HistoryOperation `json:"operation"`
	Cid       string           `json:"cid"`
	Nullified bool             `json:"nullified"`
	CreatedAt time.Time        `json:"createdAt"`
}

type HistoryOperation struct {
	Sig      string `json:"sig"`
	Prev     any    `json:"prev"`
	Type     string `json:"type"`
	Services struct {
		AtprotoPds AtprotoPDS `json:"atproto_pds"`
	} `json:"services"`
	AlsoKnownAs         []string `json:"alsoKnownAs"`
	RotationKeys        []string `json:"rotationKeys"`
	VerificationMethods struct {
		Atproto string `json:"atproto"`
	} `json:"verificationMethods"`
}

type AtprotoPDS struct {
	Type     string `json:"type"`
	Endpoint string `json:"endpoint"`
}

type Error struct {
	Err     string `json:"error"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("status=%d error=%q message=%q", e.Status, e.Err, e.Message)
}

type stringable string

func (s stringable) String() string { return string(s) }

func recordCacheKey(repo syntax.DID, collection syntax.NSID, rkey syntax.RecordKey) stringable {
	key := fmt.Sprintf("%s/%s/%s", repo, collection, rkey)
	return stringable(key)
}

func getEnv(key string, defaults ...string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		for _, val := range defaults {
			if len(val) > 0 {
				return val
			}
		}
		return ""
	}
	return v
}
