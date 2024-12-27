package sqlite

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

type Config struct {
	ReadOnly    bool
	Cache       CacheMode
	JournalMode string
	// WalCheckpoint is only used when JournalMode is "WAL"
	WalCheckpoint *int
	Pragmas       map[string]any
	Debug         bool
}

type Option func(c *Config)

// JournalMode sets the database's journal_mode pragma.
func JournalMode(mode string) Option { return func(c *Config) { c.JournalMode = mode } }

// WalCheckpoint sets the database's wal_checkpoint pragma.
func WalCheckpoint(n int) Option { return func(c *Config) { c.WalCheckpoint = &n } }

// Pragma will add a database pragma.
//
// Calling Pragma("journal_mode", "WAL") will end up executing
// PRAGMA journal_mode = WAL;
func WithPragma(name string, value any) Option {
	return func(c *Config) {
		if c.Pragmas == nil {
			c.Pragmas = make(map[string]any)
		}
		c.Pragmas[name] = value
	}
}

func ReadOnly(c *Config) { c.ReadOnly = true }

func Cache(mode CacheMode) Option         { return WithCacheMode(mode) }
func WithCacheMode(mode CacheMode) Option { return func(c *Config) { c.Cache = mode } }

// CacheMode is used to configure the 'cache' URI parameter.
//
// See https://www.sqlite.org/uri.html
type CacheMode uint8

const (
	// CacheModeNone is the default value and will not affect the connect URI.
	CacheModeNone CacheMode = iota
	// CacheModeShared will be used to add the URI query parameter '?cache=shared'.
	CacheModeShared
	// CacheModeShared will be used to add the URI query parameter '?cache=private'.
	CacheModePrivate
)

func (c *Config) query() (url.Values, error) {
	q := make(url.Values)
	if c == nil {
		return q, nil
	}
	if c.ReadOnly {
		q.Set("mode", "ro")
		q.Set("immutable", "true")
	}
	switch c.Cache {
	case CacheModeNone:
	case CacheModeShared:
		q.Set("cache", "shared")
	case CacheModePrivate:
		q.Set("cache", "private")
	default:
		return q, errors.Errorf("invalid cache configuration %d", c.Cache)
	}
	if c.Debug {
		slog.Debug("sqlite: database URI query built",
			"string", q.Encode(),
			"raw", fmt.Sprintf("%#v", q))
	}
	return q, nil
}

func (c *Config) pragmas(db *sql.DB) (err error) {
	if len(c.JournalMode) > 0 {
		err = pragma(c, db, PragmaJournalMode, strings.ToUpper(c.JournalMode))
		if err != nil {
			return err
		}
		switch strings.ToLower(c.JournalMode) {
		case "wal":
			if c.WalCheckpoint != nil {
				err = pragma(c, db, PragmaWalCheckpoint, *c.WalCheckpoint)
				if err != nil {
					return err
				}
			}
		}
	}
	for name, value := range c.Pragmas {
		err = pragma(c, db, name, value)
		if err != nil {
			return err
		}
	}
	return nil
}
