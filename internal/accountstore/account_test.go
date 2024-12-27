package accountstore

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestGetAccount(t *testing.T) {
	is := is.New(t)
	var q strings.Builder
	err := buildSelectAccount(nil, "actor.did = ?", &q)
	is.NoErr(err)
	is.True(strings.Contains(q.String(), `actor."takedownRef" is null`))
	is.True(strings.Contains(q.String(), `actor."deactivatedAt" is null`))
	q.Reset()
	err = buildSelectAccount(new(GetAccountOpts).WithDeactivated(), "actor.handle = ?", &q)
	is.NoErr(err)
	is.True(strings.Contains(q.String(), `actor."takedownRef" is null`))
	is.True(!strings.Contains(q.String(), `actor."deactivatedAt" is null`))
	q.Reset()
	err = buildSelectAccount(new(GetAccountOpts).
		WithDeactivated().
		WithTakenDown(), "actor.handle = ?", &q)
	is.NoErr(err)
	is.True(!strings.Contains(q.String(), `actor."takedownRef" is null`))
	is.True(!strings.Contains(q.String(), `actor."deactivatedAt" is null`))
}
