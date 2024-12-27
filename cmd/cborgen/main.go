package main

import (
	"log"

	cbg "github.com/whyrusleeping/cbor-gen"

	"github.com/harrybrwn/at/api/com/atproto"
	"github.com/harrybrwn/at/internal/repo"
)

func main() {
	cfg := cbg.Gen{
		MaxStringLength: 1_000_000,
	}
	err := cfg.WriteMapEncodersToFile(
		"api/com/atproto/cbor.go",
		"atproto",
		atproto.RepoStrongRef{},
		atproto.SyncSubscribeReposAccount{},
		// atproto.SyncSubscribeReposCommit{},
		atproto.SyncSubscribeReposHandle{},
		atproto.SyncSubscribeReposIdentity{},
		atproto.SyncSubscribeReposInfo{},
		atproto.SyncSubscribeReposMigrate{},
		atproto.SyncSubscribeReposRepoOp{},
		atproto.SyncSubscribeReposTombstone{},
	)
	if err != nil {
		log.Fatal(err)
	}
	err = cfg.WriteMapEncodersToFile(
		"internal/repo/cbor.go",
		"repo",
		repo.SignedCommit{},
		repo.UnsignedCommit{},
		// repo.PreparedCreate{},
		// repo.PreparedUpdate{},
		// repo.PreparedDelete{},
	)
	if err != nil {
		log.Fatal(err)
	}
}
