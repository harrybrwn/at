package xrpc

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"path"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/coder/websocket"
	"github.com/fxamacker/cbor/v2"
	"github.com/go-chi/chi/v5"
	"github.com/pkg/errors"
)

type Server struct {
	r chi.Router
}

func NewServer() *Server {
	return &Server{r: chi.NewRouter()}
}

// Router returns the internal router.
func (s *Server) Router() chi.Router { return s.r }

type MetaContextKey string

type RequestType int

const (
	Query RequestType = iota + 1
	Procedure
	Subscription
)

type Method interface {
	NSID() syntax.NSID
	Type() RequestType
}

type rpcMethod struct {
	nsid syntax.NSID
	typ  RequestType
}

func (m *rpcMethod) NSID() syntax.NSID { return m.nsid }
func (m *rpcMethod) Type() RequestType { return m.typ }

func httpMethod(t RequestType) string {
	switch t {
	case Query:
		return http.MethodGet
	case Procedure:
		return http.MethodPost
	case Subscription:
		return http.MethodGet // TODO is this right?
	default:
		panic(fmt.Sprintf(
			"xrpc request type \"%d\" is invalid. Use xrpc.Query or xrpc.Procedure",
			t))
	}
}

func (m *rpcMethod) infer() error {
	switch m.typ {
	case Query, Procedure, Subscription:
		return nil
	}
	inferred := ComAtprotoRequestType(m.nsid.String())
	switch inferred {
	case Query, Procedure:
		m.typ = inferred
		return nil
	default:
		return errors.New("Method.Type is required")
	}
}

func NewMethod(nsid syntax.NSID, typ RequestType) *rpcMethod {
	return &rpcMethod{nsid: nsid, typ: typ}
}

func newRPCMethod(nsid syntax.NSID, reqType RequestType) (*rpcMethod, error) {
	m := rpcMethod{nsid: nsid, typ: reqType}
	err := m.infer()
	return &m, err
}

type FromQuery interface {
	FromQuery(url.Values) error
}

type FromBody interface {
	FromBody(r io.Reader) error
}

type Request interface {
	FromQuery
	FromBody
	New() Request
}

type RPC interface {
	Method() Method
	http.Handler
}

func (s *Server) AddHandlers(appliers ...interface {
	Apply(*Server, ...func(http.Handler) http.Handler)
}) {
	for _, a := range appliers {
		a.Apply(s)
	}

	m := make(map[string]struct{})
	for _, route := range s.r.Routes() {
		for method := range route.Handlers {
			key := method + ":" + route.Pattern
			if _, ok := m[key]; ok {
				panic(fmt.Sprintf("found duplicate route %q", key))
			} else {
				m[key] = struct{}{}
			}
		}
	}
}

func (s *Server) AddHandler(method Method, handler http.Handler, middleware ...func(http.Handler) http.Handler) {
	m, err := newRPCMethod(method.NSID(), method.Type())
	if err != nil {
		panic(err)
	}
	r := s.r
	if len(middleware) > 0 {
		r = s.r.With(middleware...)
	}
	httpMethod := httpMethod(m.Type())
	path := path.Join("/xrpc", m.NSID().String())
	r.Method(
		httpMethod,
		path,
		handler,
	)
}

func (s *Server) AddRPCs(rpcs ...RPC) {
	for _, rpc := range rpcs {
		s.AddHandler(rpc.Method(), rpc)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.r.ServeHTTP(w, r)
}

func (s *Server) With(middleware ...func(http.Handler) http.Handler) *Server {
	return &Server{r: s.r.With(middleware...)}
}

func Stream[T any](ctx context.Context, c *websocket.Conn, seq iter.Seq[T]) (err error) {
	ctx = c.CloseRead(ctx)
	for item := range seq {
		err = write(ctx, c, item)
		if err != nil {
			return err
		}
	}
	return nil
}

func write[T any](ctx context.Context, c *websocket.Conn, item T) error {
	w, err := c.Writer(ctx, websocket.MessageText)
	if err != nil {
		return err
	}
	var b []byte
	b, err = cbor.Marshal(item)
	// err = json.NewEncoder(w).Encode(item)
	if err != nil {
		w.Close()
		return err
	}
	_, err = w.Write(b)
	if err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func ComAtprotoRequestType(nsid string) RequestType {
	switch nsid {
	case "com.atproto.admin.disableAccountInvites",
		"com.atproto.admin.disableInviteCodes",
		"com.atproto.admin.enableAccountInvites":
		return Procedure
	case "com.atproto.admin.getAccountInfo",
		"com.atproto.admin.getInviteCodes",
		"com.atproto.admin.getSubjectStatus":
		return Query
	case "com.atproto.admin.sendEmail":
		return Procedure
	case "com.atproto.admin.updateAccountEmail":
		return Procedure
	case "com.atproto.admin.updateAccountHandle":
		return Procedure
	case "com.atproto.admin.updateSubjectStatus":
		return Procedure
	case "com.atproto.identity.resolveHandle":
		return Query
	case "com.atproto.identity.updateHandle":
		return Procedure
	case "com.atproto.label.queryLabels":
		return Query
	case "com.atproto.moderation.createReport":
		return Procedure
	case "com.atproto.repo.applyWrites":
		return Procedure
	case "com.atproto.repo.createRecord":
		return Procedure
	case "com.atproto.repo.deleteRecord":
		return Procedure
	case "com.atproto.repo.describeRepo":
		return Query
	case "com.atproto.repo.getRecord":
		return Query
	case "com.atproto.repo.listRecords":
		return Query
	case "com.atproto.repo.putRecord",
		"com.atproto.repo.uploadBlob":
		return Procedure
	case "com.atproto.server.confirmEmail",
		"com.atproto.server.createAccount",
		"com.atproto.server.createAppPassword",
		"com.atproto.server.createInviteCode",
		"com.atproto.server.createInviteCodes",
		"com.atproto.server.createSession",
		"com.atproto.server.deleteAccount",
		"com.atproto.server.deleteSession":
		return Procedure
	case "com.atproto.server.describeServer",
		"com.atproto.server.getAccountInviteCodes",
		"com.atproto.server.getSession",
		"com.atproto.server.listAppPasswords":
		return Query
	case "com.atproto.server.refreshSession",
		"com.atproto.server.requestAccountDelete",
		"com.atproto.server.requestEmailConfirmation",
		"com.atproto.server.requestEmailUpdate",
		"com.atproto.server.requestPasswordReset",
		"com.atproto.server.reserveSigningKey",
		"com.atproto.server.resetPassword",
		"com.atproto.server.revokeAppPassword",
		"com.atproto.server.updateEmail":
		return Procedure
	case "com.atproto.sync.getBlob",
		"com.atproto.sync.getBlocks",
		"com.atproto.sync.getCheckout",
		"com.atproto.sync.getHead",
		"com.atproto.sync.getLatestCommit",
		"com.atproto.sync.getRecord",
		"com.atproto.sync.getRepo",
		"com.atproto.sync.listBlobs",
		"com.atproto.sync.listRepos":
		return Query
	case "com.atproto.sync.notifyOfUpdate",
		"com.atproto.sync.requestCrawl":
		return Procedure
	case "com.atproto.temp.fetchLabels":
		return Query
	default:
		return RequestType(-1)
	}
}
