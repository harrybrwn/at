package main

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/harrybrwn/env"
	"github.com/spf13/cobra"

	"github.com/harrybrwn/at/internal/actorstore"
	"github.com/harrybrwn/at/internal/middleware"
	"github.com/harrybrwn/at/internal/pds"
	"github.com/harrybrwn/at/internal/sequencer"
	"github.com/harrybrwn/at/pubsub"
	"github.com/harrybrwn/at/xrpc"
)

func newServerCmd() *cobra.Command {
	var (
		conf = pds.EnvConfig{
			Port:    3000,
			DevMode: true, // TODO Change this later
		}
		asBluesky = true
	)
	c := cobra.Command{
		Use:   "server",
		Short: "Start a small test server",
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				ctx    = cmd.Context()
				logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
					AddSource: false,
					Level:     slog.LevelDebug,
				}))
				s = xrpc.NewServer()
			)
			slog.SetDefault(logger)
			s.Router().Use(middleware.NewRequestLogger(logger))
			s.Router().Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				fmt.Fprintf(w, `This is an AT Protocol Personal Data Server (PDS): https://github.com/harrybrwn/at

Most API routes are under /xrpc/`)
			}))

			err := env.ReadEnvPrefixed("pds", &conf)
			if err != nil {
				return err
			}
			if err = conf.Validate(); err != nil {
				return err
			}
			if asBluesky {
				conf.BlueskyDefaults()
			}
			conf.InitDefaults()
			if len(conf.PlcRotationKey.K256PrivateKeyHex) == 0 {
				conf.PlcRotationKey.K256PrivateKeyHex = generateRotationKey()
				logger.Warn("generated temporary plc rotation key, this should not be done in production")
			}
			_ = os.MkdirAll(conf.DataDirectory, 0755)
			accounts, err := pds.NewAccountStore(&conf)
			if err != nil {
				return err
			}
			defer accounts.Close()
			err = accounts.Migrate(ctx)
			if err != nil {
				return err
			}

			pds, err := pds.New(
				&conf,
				logger,
				&actorstore.ActorStore{Dir: conf.ActorStore.Directory},
				accounts,
				pubsub.NewMemoryBus[*sequencer.Event[*pds.Event]](),
			)
			if err != nil {
				return err
			}
			pds.Passthrough = xrpc.NewClient(xrpc.WithEnv(), xrpc.WithURL(conf.BskyAppView.URL))
			routes(s, pds)
			logger.Info("starting server", "port", conf.Port)
			if conf.DevMode {
				logger.Warn("running pds server in dev mode")
			}
			return http.ListenAndServe(fmt.Sprintf(":%d", conf.Port), s)
		},
	}
	c.Flags().Uint16VarP(&conf.Port, "port", "p", conf.Port, "server port")
	c.Flags().StringVar(&conf.DataDirectory, "data-dir", conf.DataDirectory, "directory for all data")
	c.Flags().BoolVarP(&conf.DevMode, "dev", "D", conf.DevMode, "run server in development mode")
	c.Flags().BoolVar(&asBluesky, "not-bsky", asBluesky, "toggle default settings for connecting to bluesky")
	return &c
}

func routes(srv *xrpc.Server, p *pds.PDS) {
	srv.AddHandlers(
		// proxies all requests out to the app view
		//pds.NewPassthrough(p.Passthrough),
		// New override with my routes
		p,
	)
}

func generateRotationKey() string {
	k, err := crypto.GeneratePrivateKeyK256()
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(k.Bytes())
}
