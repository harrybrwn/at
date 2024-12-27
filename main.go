package main

//go:generate go run ./cmd/lexgen -f -d ./lexicons/ -o ./api/
//go:generate go run ./cmd/cborgen

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/harrybrwn/xdg"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/harrybrwn/at/atp"
	"github.com/harrybrwn/at/internal/pds"
)

func main() {
	root := NewRootCmd()
	err := root.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func NewRootCmd() *cobra.Command {
	var (
		cacheDisabled bool
		ctx           = newContext()
	)
	c := cobra.Command{
		Use:           "at [at://<resource>]",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if err := ctx.init(cmd.Context()); err != nil {
				return err
			}
			cache := ctx.cache
			if cacheDisabled {
				slog.Info("cache is disabled")
				ctx.dir.(*cacheDirectory).Disable()
				cache.Disable()
			}
			for _, arg := range args {
				err := view(ctx.WithCtx(cmd.Context()), arg, cache)
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
		newCacheCmd(),
		newTestCmd(),
		newLexCmd(),
		newFireHoseCmd(),
		newServerCmd(),
		newResolveCmd(),
	)
	c.Flags().BoolVarP(&ctx.verbose, "verbose", "v", ctx.verbose, "verbose output")
	c.Flags().BoolVarP(&ctx.history, "history", "H", ctx.history, "show did history")
	c.PersistentFlags().BoolVar(&ctx.purge, "purge", ctx.purge, "purge cache when before doing a resource lookup")
	c.PersistentFlags().StringVar(&ctx.cursor, "cursor", "", "cursor for fetching lists")
	c.PersistentFlags().IntVar(&ctx.limit, "limit", ctx.limit, "limit when fetching lists")
	c.PersistentFlags().BoolVar(&cacheDisabled, "no-cache", false, "disable caching on disk")
	return &c
}

func view(c *Context, arg string, cache *fileCache) error {
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
	did := ident.DID
	client := XRPCClient{pds: ident.PDSEndpoint(), cache: cache}
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
			record, err := client.Record(c.ctx, did, collection, rkey)
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
		handle := ident.Handle
		fmt.Printf("did:      %s\n", did)
		fmt.Printf("alias:    %s\n", handle)
		fmt.Printf("endpoint: %s\n", ident.PDSEndpoint())
		repo, err := client.Repo(c.ctx, did)
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
	}
	return nil
}

func NewDIDCmd(ctx *Context) *cobra.Command {
	c := cobra.Command{
		Use:   "did",
		Short: "Get a DID given a handle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// did, err := ctx.resolveHandle(cmd.Context(), args[0])
			// if err != nil {
			// 	return err
			// }
			// cmd.Println(did)
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
				client := XRPCClient{pds: ident.PDSEndpoint(), cache: ctx.cache}
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

func newResolveCmd() *cobra.Command {
	var conf pds.EnvConfig
	c := cobra.Command{
		Use:  "resolve",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			conf.InitDefaults()
			conf.BlueskyDefaults()
			resolver := atp.Resolver{HttpClient: http.DefaultClient}
			resolver.HandleResolver, err = atp.NewDefaultHandleResolver()
			if err != nil {
				return err
			}
			if len(conf.DidPlcURL) == 0 {
				return errors.New("no plc url given")
			}
			resolver.PlcURL, err = url.Parse(conf.DidPlcURL)
			if err != nil {
				return err
			}
			fmt.Printf("%#v\n", resolver.PlcURL)
			doc, err := resolver.GetDocument(cmd.Context(), args[0])
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

func newCacheCmd() *cobra.Command {
	c := cobra.Command{
		Use:   "cache",
		Short: "Manage the disk cache",
	}
	c.AddCommand(
		&cobra.Command{
			Use: "clear", Aliases: []string{"purge"}, Short: "Completely purge the disk cache",
			RunE: func(cmd *cobra.Command, args []string) error { return os.RemoveAll(xdg.Cache("at")) },
		},
	)
	return &c
}

func newTestCmd() *cobra.Command {
	c := cobra.Command{
		Use:    "test",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// base := "/tmp/test-cache"
			base := args[0]
			cleaner := cleaner{basedir: base}
			cache := fileCache{dir: base, cleaner: &cleaner}
			cache.clean()
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
