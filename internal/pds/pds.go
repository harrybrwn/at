package pds

import (
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/indigo/events"
	"github.com/bluesky-social/indigo/plc"

	atpapi "github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/atp"
	"github.com/harrybrwn/at/internal/accountstore"
	"github.com/harrybrwn/at/internal/actorstore"
	"github.com/harrybrwn/at/internal/auth"
	"github.com/harrybrwn/at/internal/repo"
	"github.com/harrybrwn/at/internal/sequencer"
	"github.com/harrybrwn/at/xrpc"
)

type PDS struct {
	logger         *slog.Logger
	cfg            *EnvConfig
	ActorStore     *actorstore.ActorStore
	Accounts       *accountstore.AccountStore
	Passthrough    *xrpc.Client
	Resolver       atp.DidResolver
	PLC            plc.PLCClient
	Events         *events.EventManager
	Bus            sequencer.Bus[*Event]
	plcRotationKey *crypto.PrivateKeyK256
}

func New(
	config *EnvConfig,
	logger *slog.Logger,
	actorstore *actorstore.ActorStore,
	accounts *accountstore.AccountStore,
	bus sequencer.Bus[*Event],
) (*PDS, error) {
	plcRotKeyRaw, err := hex.DecodeString(config.PlcRotationKey.K256PrivateKeyHex)
	if err != nil {
		return nil, err
	}
	plcRotationKey, err := crypto.ParsePrivateBytesK256(plcRotKeyRaw)
	if err != nil {
		return nil, err
	}
	resolver := atp.Resolver{HttpClient: http.DefaultClient}
	resolver.HandleResolver, err = atp.NewDefaultHandleResolver()
	if err != nil {
		return nil, err
	}
	resolver.PlcURL, err = url.Parse(config.DidPlcURL)
	if err != nil {
		return nil, err
	}
	seq, err := sequencer.New(config.SequencerDBLocation, bus)
	if err != nil {
		return nil, err
	}
	pds := PDS{
		logger:         logger,
		cfg:            config,
		ActorStore:     actorstore,
		Accounts:       accounts,
		Resolver:       &resolver,
		Bus:            seq,
		plcRotationKey: plcRotationKey,
	}
	if config.DevMode {
		pds.PLC = &atp.FakePLC{Resolver: &resolver}
		pds.Resolver = accountstore.NewResolver(pds.Accounts, config.Hostname)
	} else {
		pds.PLC = &atp.PLC{Resolver: &resolver}
	}
	return &pds, nil
}

func (pds *PDS) Apply(srv *xrpc.Server, middleware ...func(http.Handler) http.Handler) {
	adminOnly := auth.AdminOnly(&auth.Opts{
		Logger:        pds.logger,
		JWTSecret:     []byte(pds.cfg.JwtSecret),
		AdminPassword: pds.cfg.AdminPassword,
	})
	authRequired := auth.Required(&auth.Opts{
		Logger:        pds.logger,
		JWTSecret:     []byte(pds.cfg.JwtSecret),
		AdminPassword: pds.cfg.AdminPassword,
	})
	srv.With(adminOnly).AddRPCs(
		atpapi.NewServerCreateInviteCodeHandler(pds),
	)
	srv.With(authRequired).AddRPCs(
		atpapi.NewRepoApplyWritesHandler(pds),
		atpapi.NewRepoCreateRecordHandler(pds),
		atpapi.NewRepoDeleteRecordHandler(pds),
		atpapi.NewRepoPutRecordHandler(pds),
		atpapi.NewRepoUploadBlobHandler(pds),
		atpapi.NewServerCreateAccountHandler(pds),
	)
	srv.AddHandlers(
		atpapi.NewRepoDescribeRepoHandler(pds),
		atpapi.NewRepoGetRecordHandler(pds),
		atpapi.NewRepoImportRepoHandler(pds),
		atpapi.NewRepoListMissingBlobsHandler(pds),
		atpapi.NewRepoListRecordsHandler(pds),
		// atpapi.NewServerConfirmEmailHandler(pds),
		atpapi.NewServerCreateSessionHandler(pds),
		// atpapi.NewServerDeleteAccountHandler(pds),
		// atpapi.NewServerDeleteSessionHandler(pds),
		atpapi.NewServerDescribeServerHandler(pds),
		// atpapi.NewServerGetSessionHandler(pds),
		// atpapi.NewServerRefreshSessionHandler(pds),
		// atpapi.NewServerUpdateEmailHandler(pds),
	)
}

func (pds *PDS) newBlobstore(did syntax.DID) *repo.DiskBlobStore {
	if pds.cfg.BlobstoreDisk != nil {
		return repo.NewDiskBlobStore(
			did.String(),
			pds.cfg.BlobstoreDisk.Location,
			pds.cfg.BlobstoreDisk.TmpLocation,
			"",
		)
	}
	return nil
}
