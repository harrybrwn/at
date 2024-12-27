package actorstore

import (
	"cmp"
	"context"
	"encoding/json"
	"io"
	"iter"
)

type PreferenceReader datastore

type Preference struct {
	ID    int
	Name  string
	Value map[string]any
}

func (pr *PreferenceReader) Get(ctx context.Context, name string) (*Preference, error) {
	r, err := pr.db.QueryContext(ctx, `SELECT id, name, valueJson FROM account_pref WHERE name = ?`, name)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var p Preference
	return &p, scanPref(r, &p)
}

func (pr *PreferenceReader) List(ctx context.Context) ([]Preference, error) {
	rows, err := pr.db.QueryContext(ctx, `SELECT id, name, valueJson FROM account_pref`)
	if err != nil {
		return nil, err
	}
	res := make([]Preference, 0)
	for rows.Next() {
		var p Preference
		err = scanPref(rows, &p)
		if err != nil {
			rows.Close()
			return nil, err
		}
		res = append(res, p)
	}
	return res, rows.Close()
}

type scanner interface {
	Scan(...any) error
	Next() bool
	io.Closer
}

type scannable interface {
	Scan(interface{ Scan(...any) error }) error
}

func basicIterScan[T cmp.Ordered](scanner scanner) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		defer scanner.Close()
		for scanner.Next() {
			var v T
			err := scanner.Scan(&v)
			if err != nil {
				yield(v, err)
				return
			}
			if !yield(v, nil) {
				return
			}
		}
	}
}

func iterScanner[T scannable](scanner scanner) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		defer scanner.Close()
		for scanner.Next() {
			var v T
			err := v.Scan(scanner)
			if err != nil {
				yield(&v, err)
				return
			}
			if !yield(&v, nil) {
				return
			}
		}
	}
}

func (p *Preference) Scan(scanner interface{ Scan(...any) error }) error {
	return scanPref(scanner, p)
}

func scanPref(row interface{ Scan(...any) error }, p *Preference) error {
	var val string
	err := row.Scan(&p.ID, &p.Name, &val)
	if err != nil {
		return err
	}
	p.Value = make(map[string]any)
	return json.Unmarshal([]byte(val), &p.Value)
}
