// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package sqlpage_test

import (
	"errors"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	"github.com/protobuf-orm/ent/dialect/sql/sqlpage"
)

// where renders what After adds to a query on an engine, which is the thing
// under test: these are predicates and the only way to read one is to build
// the statement.
func where(t *testing.T, engine string, by []sqlpage.Order, at []any) (string, []any) {
	t.Helper()

	p, err := sqlpage.After(by, at)
	require.NoError(t, err)

	s := sql.Dialect(engine).Select("*").From(sql.Table("user"))
	p(s)

	q, args := s.Query()
	_, w, _ := strings.Cut(q, " WHERE ")
	return w, args
}

func TestAfter(t *testing.T) {
	t.Run("one column reads as one comparison", func(t *testing.T) {
		x := require.New(t)

		w, args := where(t, dialect.MySql, []sqlpage.Order{{Column: "id"}}, []any{7})
		x.Equal("`user`.`id` > ?", w)
		x.Equal([]any{7}, args)

		w, _ = where(t, dialect.Postgres, []sqlpage.Order{{Column: "id"}}, []any{7})
		x.Equal(`"user"."id" > $1`, w)
	})

	t.Run("descending turns the comparison round", func(t *testing.T) {
		x := require.New(t)

		w, _ := where(t, dialect.SQLite, []sqlpage.Order{{Column: "id", Desc: true}}, []any{7})
		x.Equal("`user`.`id` < ?", w)
	})

	t.Run("one way, an engine that seeks on a row value is given one", func(t *testing.T) {
		x := require.New(t)

		// The whole of keyset paging: rows strictly past the first column, and
		// where it is equal, rows strictly past the tiebreaker. Anything
		// looser repeats a row and anything tighter skips one -- and a row
		// value is exactly that, in a form the index can start at.
		by := []sqlpage.Order{{Column: "date_created", Desc: true}, {Column: "id", Desc: true}}
		w, args := where(t, dialect.Postgres, by, []any{"t", 7})
		x.Equal(`("user"."date_created", "user"."id") < ($1, $2)`, w)
		x.Equal([]any{"t", 7}, args)

		w, _ = where(t, dialect.SQLite, []sqlpage.Order{{Column: "date_created"}, {Column: "id"}}, []any{"t", 7})
		x.Equal("(`user`.`date_created`, `user`.`id`) > (?, ?)", w)
	})

	t.Run("one way, MySQL is given the OR its ranges are made of", func(t *testing.T) {
		x := require.New(t)

		by := []sqlpage.Order{{Column: "date_created", Desc: true}, {Column: "id", Desc: true}}
		w, args := where(t, dialect.MySql, by, []any{"t", 7})
		x.Equal("`user`.`date_created` <= ? AND (`user`.`date_created` < ? OR (`user`.`date_created` = ? AND `user`.`id` < ?))", w)
		x.Equal([]any{"t", "t", "t", 7}, args)
	})

	t.Run("mixed directions are each their own, behind a bound on the first", func(t *testing.T) {
		x := require.New(t)

		// A row value compares every column one way, which is wrong here; the
		// bound is what the OR implies, and where the index starts.
		by := []sqlpage.Order{{Column: "name", Desc: true}, {Column: "id"}}
		for engine, want := range map[string]string{
			dialect.Postgres: `"user"."name" <= $1 AND ("user"."name" < $2 OR ("user"."name" = $3 AND "user"."id" > $4))`,
			dialect.SQLite:   "`user`.`name` <= ? AND (`user`.`name` < ? OR (`user`.`name` = ? AND `user`.`id` > ?))",
			dialect.MySql:    "`user`.`name` <= ? AND (`user`.`name` < ? OR (`user`.`name` = ? AND `user`.`id` > ?))",
		} {
			w, args := where(t, engine, by, []any{"a", 7})
			x.Equal(want, w, engine)
			x.Equal([]any{"a", "a", "a", 7}, args, engine)
		}
	})

	t.Run("mixed directions, the columns that lead one way bound it together", func(t *testing.T) {
		x := require.New(t)

		by := []sqlpage.Order{{Column: "kind"}, {Column: "name"}, {Column: "id", Desc: true}}
		w, args := where(t, dialect.Postgres, by, []any{"k", "a", 7})
		x.Equal(`("user"."kind", "user"."name") >= ($1, $2) AND ("user"."kind" > $3 OR ("user"."kind" = $4 AND ("user"."name" > $5 OR ("user"."name" = $6 AND "user"."id" < $7))))`, w)
		x.Equal([]any{"k", "a", "k", "k", "a", "a", 7}, args)

		// MySQL takes no row value, so its bound is the first column's.
		w, _ = where(t, dialect.MySql, by, []any{"k", "a", 7})
		x.True(strings.HasPrefix(w, "`user`.`kind` >= ? AND (`user`.`kind` > ? OR"), w)
	})

	t.Run("nothing to order by narrows nothing", func(t *testing.T) {
		x := require.New(t)

		p, err := sqlpage.After(nil, nil)
		x.NoError(err)

		s := sql.Select("*").From(sql.Table("user"))
		p(s)

		q, args := s.Query()
		x.NotContains(q, "WHERE")
		x.Empty(args)
	})

	t.Run("a cursor of the wrong width is not a cursor", func(t *testing.T) {
		x := require.New(t)

		_, err := sqlpage.After([]sqlpage.Order{{Column: "id"}}, []any{1, 2})
		x.ErrorIs(err, sqlpage.ErrCursor)
	})
}

func TestCursor(t *testing.T) {
	t.Run("comes back as what it went in as", func(t *testing.T) {
		x := require.New(t)

		at := time.Date(2026, 8, 6, 11, 22, 33, 0, time.UTC)
		id := uuid.MustParse("726f6f74-0000-0000-0000-000000000000")

		s, err := sqlpage.Encode(at, id)
		x.NoError(err)
		x.NotEmpty(s)

		var (
			gotAt time.Time
			gotId uuid.UUID
		)
		x.NoError(sqlpage.Decode(s, &gotAt, &gotId))
		x.True(at.Equal(gotAt))
		x.Equal(id, gotId)
	})

	t.Run("survives a URL", func(t *testing.T) {
		x := require.New(t)

		// A cursor is handed back in a request, and a request may travel as a
		// query string on the way to one.
		s, err := sqlpage.Encode("a/b+c?d", 1)
		x.NoError(err)
		x.NotContains(s, "/")
		x.NotContains(s, "+")
		x.NotContains(s, "=")
	})

	t.Run("what is not a cursor says so", func(t *testing.T) {
		x := require.New(t)

		var v int
		for _, s := range []string{"!!!!", "", "aGVsbG8"} {
			x.True(errors.Is(sqlpage.Decode(s, &v), sqlpage.ErrCursor), s)
		}
	})

	t.Run("one made for another order is not this one's", func(t *testing.T) {
		x := require.New(t)

		s, err := sqlpage.Encode(1, 2, 3)
		x.NoError(err)

		var a, b int
		x.ErrorIs(sqlpage.Decode(s, &a, &b), sqlpage.ErrCursor)
	})
}

func TestSize(t *testing.T) {
	x := require.New(t)

	x.Equal(10, sqlpage.Size(10, 20, 100))
	x.Equal(20, sqlpage.Size(0, 20, 100), "asking for nothing is asking for the usual")
	x.Equal(20, sqlpage.Size(-1, 20, 100), "and so is asking for nonsense")
	x.Equal(100, sqlpage.Size(1_000_000, 20, 100), "the cap is not a suggestion")
	x.Equal(100, sqlpage.Size(100, 20, 100))
}
