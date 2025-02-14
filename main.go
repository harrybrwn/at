package main

//go:generate go run ./cmd/lexgen -f -d ./lexicons/ -o ./api/
// go : generate go run ./cmd/cborgen

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/internal/auth"
	"github.com/harrybrwn/at/internal/didcache"
	"github.com/harrybrwn/at/internal/httpcache"
	"github.com/harrybrwn/at/xrpc"
)

func main() {
	root := NewRootCmd()
	err := root.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", err)
		os.Exit(1)
	}
}

func NewRootCmd() *cobra.Command {
	var (
		ctx         = newContext()
		logLevelStr = "info"
		debug       bool
	)
	c := cobra.Command{
		Use:           "at [at://<resource>]",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) (err error) {
			var lvl slog.Level
			if err = lvl.UnmarshalText([]byte(logLevelStr)); err != nil {
				return err
			}
			if debug {
				lvl = slog.LevelDebug
			}
			l := slog.New(slog.NewJSONHandler(cmd.OutOrStdout(), &slog.HandlerOptions{
				Level: lvl,
			}))
			slog.SetDefault(l)
			ctx.logger = l
			return ctx.init(cmd.Context())
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			return ctx.cleanup()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			for _, arg := range args {
				err := view(ctx.WithCtx(cmd.Context()), arg)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}

	c.AddCommand(
		NewDIDCmd(ctx),
		newBlobCmd(ctx),
		newCacheCmd(ctx),
		newTestCmd(),
		newLexCmd(),
		newFireHoseCmd(),
		newServerCmd(),
		newResolveCmd(ctx),
		newServiceJwtCmd(),
	)
	c.Flags().BoolVarP(&ctx.verbose, "verbose", "v", ctx.verbose, "verbose output")
	c.Flags().BoolVarP(&ctx.history, "history", "H", ctx.history, "show did:plc history")
	c.Flags().BoolVarP(&ctx.diddoc, "did-doc", "D", ctx.diddoc, "show did doc when calling describeRepo")
	c.PersistentFlags().BoolVar(&ctx.purge, "purge", ctx.purge, "purge cache when before doing a resource lookup")
	c.PersistentFlags().StringVar(&ctx.cursor, "cursor", "", "cursor for fetching lists")
	c.PersistentFlags().IntVar(&ctx.limit, "limit", ctx.limit, "limit when fetching lists")
	c.PersistentFlags().BoolVar(&ctx.noCache, "no-cache", ctx.noCache, "disable caching")
	c.PersistentFlags().StringVarP(&logLevelStr, "log-level", "l", logLevelStr, "set the log level (debug|info|warn|error)")
	c.PersistentFlags().BoolVarP(&debug, "debug", "d", debug, "turn on debug mode")
	return &c
}

func view(c *Context, arg string) error {
	if !strings.HasPrefix(arg, "at://") {
		arg = "at://" + arg
	}
	arg, _ = strings.CutSuffix(arg, "/")
	uri, err := syntax.ParseATURI(arg)
	if err != nil {
		return err
	}

	if c.purge {
		err = c.dir.Purge(c.ctx, uri.Authority())
		if err != nil {
			slog.Error("failed to purge cache",
				slog.String("identifier", uri.Authority().String()),
				slog.Any("error", err))
		}
	}

	ident, err := c.dir.Lookup(c.ctx, uri.Authority())
	switch {
	case errors.Is(err, identity.ErrHandleMismatch):
		if ident == nil {
			return err
		} else {
			slog.Warn("handle mismatch", slog.Any("error", err))
		}
	case err == nil:
		break
	default:
		return errors.Wrap(err, "failed to lookup handle")
	}
	ident.Handle, err = ident.DeclaredHandle()
	if err != nil {
		return errors.Wrap(err, "failed to find handle from did doc alsoKnownAs")
	}
	did := ident.DID
	client := XRPCClient{pds: ident.PDSEndpoint()}
	if v, ok := os.LookupEnv("PDS_ADMIN_PASSWORD"); ok {
		if c.verbose {
			slog.Info("setting xrpc client admin token")
		}
		client.AdminToken = &v
	}
	collection := uri.Collection()
	rkey := uri.RecordKey()

	if len(rkey) > 0 {
		switch collection {
		case "app.bsky.labeler.service":
			record, err := client.LabelerRecord(c.ctx, did, collection, rkey)
			if err != nil {
				return err
			}
			err = jsonIndent(os.Stdout, record)
			if err != nil {
				return err
			}
		case "app.bsky.actor.profile":
			record, err := client.ProfileRecord(c.ctx, did, collection, rkey)
			if err != nil {
				return err
			}
			cid, err := syntax.ParseCID(record.Value.Avatar.Ref.Link)
			if err != nil {
				return err
			}
			err = jsonIndent(os.Stdout, record)
			if err != nil {
				return err
			}
			fmt.Println()
			fmt.Println("avatar:", blobURL(client.pds, did, cid))
		default:
			cli := xrpc.NewClient(xrpc.WithEnv(), xrpc.WithURL(ident.PDSEndpoint()), xrpc.WithClient(HttpClient))
			record, err := atproto.NewRepoClient(cli).GetRecord(c.ctx, &atproto.RepoGetRecordParams{
				Repo:       &syntax.AtIdentifier{Inner: did},
				Collection: collection,
				RKey:       rkey.String(),
			})
			if err != nil {
				return err
			}
			err = jsonIndent(os.Stdout, record)
			if err != nil {
				return err
			}
		}
		fmt.Println()
	} else if len(collection) > 0 {
		switch collection {
		case "app.bsky.feed.getLikes":
			res := make(map[string]any)
			err := client.dojson(c.ctx, xrpc.Query, collection.String(), nil, res)
			if err != nil {
				return err
			}
			fmt.Println(res)

		default:
			records, err := client.ListRecords(c.ctx, did, collection, c.limit, c.cursor)
			if err != nil {
				return err
			}
			if c.verbose {
				for _, record := range records.Records {
					err = jsonIndent(os.Stdout, record)
					if err != nil {
						return err
					}
					fmt.Println()
				}
			} else {
				for _, record := range records.Records {
					fmt.Println(record.Cid, record.URI)
				}
			}
		}
	} else {
		fmt.Printf("did:      %s\n", did)
		fmt.Printf("alias:    %s\n", ident.Handle)
		fmt.Printf("endpoint: %s\n", ident.PDSEndpoint())
		cli := xrpc.NewClient(xrpc.WithEnv(), xrpc.WithURL(ident.PDSEndpoint()), xrpc.WithClient(HttpClient))
		repo, err := atproto.NewRepoClient(cli).DescribeRepo(c.ctx, &atproto.RepoDescribeRepoParams{
			Repo: &syntax.AtIdentifier{Inner: did},
		})
		if err != nil {
			return err
		}
		fmt.Printf("collections:\n")
		for _, cl := range repo.Collections {
			fmt.Printf("  - %s\n", cl)
		}

		if c.history {
			history, err := PlcHistory(did)
			if err != nil {
				return err
			}
			fmt.Printf("history:\n")
			for _, item := range history {
				fmt.Printf("  - did:        %s\n", item.DID)
				fmt.Printf("    cid:        %s\n", item.Cid)
				fmt.Printf("    nullified:  %v\n", item.Nullified)
				fmt.Printf("    created at: %v\n", item.CreatedAt)
				fmt.Printf("    operation:\n")
				fmt.Printf("      type:          %s\n", item.Operation.Type)
				fmt.Printf("      prev:          %v\n", item.Operation.Prev)
				fmt.Printf("      sig:           %s\n", item.Operation.Sig)
				fmt.Printf("      also known as: %s\n", item.Operation.AlsoKnownAs)
				fmt.Printf("      rot keys:      %s\n", item.Operation.RotationKeys)
				fmt.Printf("      verification:  %+v\n", item.Operation.VerificationMethods)
				fmt.Printf("      services:      %+v\n", item.Operation.Services)
			}
		}
		if c.diddoc {
			indentedDidDoc, err := json.MarshalIndent(repo.DidDoc, "", "  ")
			if err != nil {
				return err
			}
			fmt.Printf("%s\n", indentedDidDoc)
		}
	}
	return nil
}

func NewDIDCmd(cx *Context) *cobra.Command {
	c := cobra.Command{
		Use:   "did",
		Short: "Get a DID given a handle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := cx.init(ctx); err != nil {
				return err
			}
			at, err := syntax.ParseAtIdentifier(args[0])
			if err != nil {
				return err
			}
			if at.IsDID() {
				did, err := at.AsDID()
				if err != nil {
					return err
				}
				fmt.Println(did)
				return nil
			}
			h, err := at.AsHandle()
			if err != nil {
				return err
			}
			did, err := cx.dir.LookupHandle(ctx, h)
			if err != nil {
				return err
			}
			fmt.Printf("%s\n", did.DID)
			return nil
		},
	}
	return &c
}

func newBlobCmd(ctx *Context) *cobra.Command {
	var open bool
	c := cobra.Command{
		Use:     "blob <at-identifier> [cid]",
		Aliases: []string{"blobs"},
		Short:   "Inspect blobs",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ctx.init(cmd.Context()); err != nil {
				return err
			}
			arg := args[0]
			if !strings.HasPrefix(arg, "at://") {
				arg = "at://" + arg
			}
			arg, _ = strings.CutSuffix(arg, "/")
			uri, err := syntax.ParseATURI(arg)
			if err != nil {
				return err
			}

			if ctx.purge {
				err := ctx.dir.Purge(ctx.ctx, uri.Authority())
				if err != nil {
					slog.Error("failed to purge cache",
						slog.String("identifier", uri.Authority().String()),
						slog.Any("error", err))
				}
			}
			ident, err := ctx.dir.Lookup(ctx.ctx, uri.Authority())
			switch {
			case errors.Is(err, identity.ErrHandleMismatch):
				if ident == nil {
					return err
				} else {
					slog.Warn("handle mismatch", slog.Any("error", err))
				}
			case err == nil:
				break
			default:
				return err
			}
			if len(args) > 1 {
				cid, err := syntax.ParseCID(args[1])
				if err != nil {
					return err
				}
				u := blobURL(ident.PDSEndpoint(), ident.DID, cid)
				if open {
					return exec.Command("xdg-open", u).Run()
				} else {
					fmt.Println(u)
				}
			} else {
				client := XRPCClient{pds: ident.PDSEndpoint()}
				blobs, err := client.ListBlobs(ctx.ctx, ident.DID, ctx.limit, ctx.cursor)
				if err != nil {
					return err
				}
				for _, cid := range blobs.CIDs {
					fmt.Printf("\"%[3]s/xrpc/com.atproto.sync.getBlob?did=%[1]s&cid=%[2]s\"\n", ident.DID, cid, client.pds)
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&open, "open", false, "open urls in a browser")
	return &c
}

func newResolveCmd(cx *Context) *cobra.Command {
	c := cobra.Command{
		Use:  "resolve",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if err = cx.init(cmd.Context()); err != nil {
				return err
			}
			doc, err := cx.resolver.GetDocument(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			b, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", b)
			return nil
		},
	}
	return &c
}

func newServiceJwtCmd() *cobra.Command {
	var (
		key          string
		checkWithPlc bool
		multibase    bool
		exp          time.Duration
		lxm          string
	)
	c := cobra.Command{
		Use:   "service-jwt",
		Short: "Generate a service JWT given a DID and a private Key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			did := args[0]
			rawkey, err := os.ReadFile(key)
			if err != nil {
				return err
			}
			key, err := crypto.ParsePrivateBytesK256(rawkey)
			if err != nil {
				return err
			}
			pubkey, err := key.PublicKey()
			if err != nil {
				return err
			}
			opts := auth.ServiceJwtOpts{
				Iss:     did,
				Aud:     did,
				KeyPair: key,
			}
			if exp > 0 {
				expiration := time.Now().Add(exp)
				opts.Exp = &expiration
			}
			token, err := auth.CreateServiceJwt(&opts)
			if err != nil {
				return err
			}
			if multibase {
				fmt.Fprintf(cmd.OutOrStdout(), "private key multibase: %s\n", key.Multibase())
				fmt.Fprintf(cmd.OutOrStdout(), "public key multibase:  %s\n", pubkey.Multibase())
				fmt.Fprintf(cmd.OutOrStdout(), "\n")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", token)
			return nil
		},
	}
	c.Flags().StringVarP(&key, "key", "k", key, "filepath to a private key")
	c.Flags().BoolVarP(&multibase, "multibase", "m", multibase, "print the multibase public key as well")
	c.Flags().BoolVar(&checkWithPlc, "check-with-plc", checkWithPlc, "Verify that the private key matches the public key stored in a public PLC server")
	c.Flags().DurationVarP(&exp, "expiration", "e", exp, "set the expiration duration")
	c.Flags().StringVarP(&lxm, "lexicon-method", "L", lxm, "lexicon method to use inside the jwt")
	return &c
}

func newCacheCmd(cx *Context) *cobra.Command {
	c := cobra.Command{
		Use:   "cache",
		Short: "Manage cached data.",
	}
	c.AddCommand(
		&cobra.Command{
			Use: "clear", Aliases: []string{"purge"}, Short: "Completely purge the cache",
			RunE: func(cmd *cobra.Command, args []string) error {
				ctx := cmd.Context()
				err := cx.init(ctx)
				if err != nil {
					return err
				}
				err = didcache.Purge(ctx, cx.cacheDB)
				if err != nil {
					return err
				}
				err = httpcache.Purge(ctx, cx.cacheDB)
				if err != nil {
					return err
				}
				return nil
			},
		},
	)
	return &c
}

func newTestCmd() *cobra.Command {
	c := cobra.Command{
		Use:    "test",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	return &c
}

func jsonIndent(w io.Writer, v any) error {
	blob, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(blob)
	return err
}

// https://github.com/notjuliet/pdsls/blob/main/src/views/record.tsx#L156
func contentLink(uri syntax.ATURI) string {
	switch uri.Collection() {
	case "app.bsky.actor.profile":
		return fmt.Sprintf("https://bsky.app/profile/%s", uri.Authority())
	case "app.bsky.feed.post":
		return fmt.Sprintf("https://bsky.app/profile/%s/post/%s", uri.Authority(), uri.RecordKey())
	case "app.bsky.feed.generator":
		return fmt.Sprintf("https://bsky.app/profile/%s/feed/%s", uri.Authority(), uri.RecordKey())
	case "app.bsky.graph.list":
		return fmt.Sprintf("https://bsky.app/profile/%s/lists/%s", uri.Authority(), uri.RecordKey())
	case "blue.linkat.board":
		return fmt.Sprintf("https://linkat.blue/%s", uri.Authority())
	default:
		return ""
	}
}

func urlOpen(u string) error {
	return exec.Command("xdg-open", u).Run()
}
