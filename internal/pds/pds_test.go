package pds

import (
	"log/slog"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/harrybrwn/at/internal/actorstore"
	"github.com/harrybrwn/at/internal/sequencer"
	"github.com/harrybrwn/at/pubsub"
)

type testConfOption func(cfg *EnvConfig)

func localhost(cfg *EnvConfig) { cfg.Hostname = "localhost" }

func withHost(h string) testConfOption {
	return func(cfg *EnvConfig) { cfg.Hostname = h }
}

func testPDS(t *testing.T, opts ...testConfOption) *PDS {
	t.Helper()
	conf := EnvConfig{
		DevMode:       true,
		LogEnabled:    true,
		LogLevel:      "debug",
		DataDirectory: t.TempDir(),
		JwtSecret:     "fe62fcf606785c916f265548c39a3628",
		BlobstoreDisk: &EnvBlobstoreDisk{
			Location:    filepath.Join(t.TempDir(), "blobs"),
			TmpLocation: filepath.Join(t.TempDir(), "tmp-blobs"),
		},
	}
	for _, o := range opts {
		o(&conf)
	}
	conf.InitDefaults()
	if len(conf.ActorStore.Directory) == 0 {
		t.Error("should have actor store dir")
	}
	conf.PlcRotationKey.K256PrivateKeyHex = "646a6d121fcbe562cdcc2446efa5f26542e475a80fd71adee9adf626198fd508"
	pds, err := New(
		&conf,
		slog.Default(),
		&actorstore.ActorStore{Dir: conf.ActorStore.Directory, ReadOnly: false},
		must(NewAccountStore(&conf)),
		pubsub.NewMemoryBus[*sequencer.Event[*Event]](),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = pds.Accounts.Migrate(t.Context())
	if err != nil {
		panic(err)
	}
	return pds
}

func must[T any](v T, e error) T {
	if e != nil {
		panic(e)
	}
	return v
}
