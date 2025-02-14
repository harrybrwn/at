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

	appbsky "github.com/harrybrwn/at/api/app/bsky"
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
	pipethrough    *xrpc.Pipethrough
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
		pipethrough: &xrpc.Pipethrough{
			Host:   config.BskyAppView.URLHost(),
			Client: resolver.HttpClient,
			Logger: logger,
		},
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
	opts := auth.Opts{
		Logger:        pds.logger,
		JWTSecret:     []byte(pds.cfg.JwtSecret),
		AdminPassword: pds.cfg.AdminPassword,
		Resolver:      pds.Resolver,
	}
	adminOnly := auth.AdminOnly(&opts)
	authRequired := auth.Required(&opts)
	refreshTokenRequired := auth.RefreshTokenOnly(&opts)
	srv.With(adminOnly).AddRPCs(
		atpapi.NewServerCreateInviteCodeHandler(pds),
	)
	serviceJwt := auth.ServiceJwt(&opts)
	srv.With(authRequired).AddRPCs(
		appbsky.NewActorGetProfileHandler(pds),
		appbsky.NewNotificationListNotificationsHandler(pds),
		atpapi.NewRepoApplyWritesHandler(pds),
		atpapi.NewRepoCreateRecordHandler(pds),
		atpapi.NewRepoDeleteRecordHandler(pds),
		atpapi.NewRepoListMissingBlobsHandler(pds),
		atpapi.NewRepoPutRecordHandler(pds),
		atpapi.NewRepoUploadBlobHandler(pds),
	)
	srv.With(serviceJwt).AddRPCs(
		atpapi.NewServerCreateAccountHandler(pds),
	)
	srv.AddHandler(
		xrpc.NewMethod("app.bsky.actor.getProfile", xrpc.Query),
		pds.pipethrough,
	)
	srv.AddHandlers(
		atpapi.NewRepoDescribeRepoHandler(pds),
		atpapi.NewRepoGetRecordHandler(pds),
		atpapi.NewRepoImportRepoHandler(pds),
		atpapi.NewRepoListRecordsHandler(pds),
		// atpapi.NewServerConfirmEmailHandler(pds),
		atpapi.NewServerCreateSessionHandler(pds),
		// atpapi.NewServerDeleteAccountHandler(pds),
		// atpapi.NewServerDeleteSessionHandler(pds),
		atpapi.NewServerDescribeServerHandler(pds),
		// atpapi.NewServerGetSessionHandler(pds),
		// atpapi.NewServerUpdateEmailHandler(pds),
	)
	srv.With(refreshTokenRequired).AddHandlers(
		atpapi.NewServerRefreshSessionHandler(pds),
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
