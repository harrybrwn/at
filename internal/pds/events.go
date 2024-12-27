package pds

import (
	"time"

	"github.com/harrybrwn/at/api/com/atproto"
)

// type Event repo.Event[atproto.SyncSubscribeReposUnion]

type Event atproto.SyncSubscribeReposUnion

func (e *Event) SetSeq(seq int64) {
	switch {
	case e.SyncSubscribeReposCommit != nil:
		e.SyncSubscribeReposCommit.Seq = seq
	case e.SyncSubscribeReposIdentity != nil:
		e.SyncSubscribeReposIdentity.Seq = seq
	case e.SyncSubscribeReposAccount != nil:
		e.SyncSubscribeReposAccount.Seq = seq
	case e.SyncSubscribeReposHandle != nil:
		e.SyncSubscribeReposHandle.Seq = seq
	case e.SyncSubscribeReposMigrate != nil:
		e.SyncSubscribeReposMigrate.Seq = seq
	case e.SyncSubscribeReposTombstone != nil:
		e.SyncSubscribeReposTombstone.Seq = seq
	case e.SyncSubscribeReposInfo != nil:
	}
}

func (e *Event) SetTime(tm time.Time) {
	t := tm.Format(time.RFC3339)
	switch {
	case e.SyncSubscribeReposCommit != nil:
		e.SyncSubscribeReposCommit.Time = t
	case e.SyncSubscribeReposIdentity != nil:
		e.SyncSubscribeReposIdentity.Time = t
	case e.SyncSubscribeReposAccount != nil:
		e.SyncSubscribeReposAccount.Time = t
	case e.SyncSubscribeReposHandle != nil:
		e.SyncSubscribeReposHandle.Time = t
	case e.SyncSubscribeReposMigrate != nil:
		e.SyncSubscribeReposMigrate.Time = t
	case e.SyncSubscribeReposTombstone != nil:
		e.SyncSubscribeReposTombstone.Time = t
	case e.SyncSubscribeReposInfo != nil:
	}
}
