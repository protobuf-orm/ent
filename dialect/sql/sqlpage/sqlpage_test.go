// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package sqlpage_test

import (
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	"github.com/protobuf-orm/ent/dialect/sql/sqlpage"
)

// where renders what After adds to a query, which is the thing under test:
// these are predicates and the only way to read one is to build the statement.
func where(t *testing.T, by []sqlpage.Order, at []any) (string, []any) {
	t.Helper()

	p, err := sqlpage.After(by, at)
	require.NoError(t, err)

	s := sql.Select("*").From(sql.Table("user"))
	p(s)

	q, args := s.Query()
	return q, args
}

func TestAfter(t *testing.T) {
	t.Run("one column reads as one comparison", func(t *testing.T) {
		x := require.New(t)

		q, args := where(t, []sqlpage.Order{{Column: "id"}}, []any{7})
		x.Contains(q, "`user`.`id` > ?")
		x.Equal([]any{7}, args)
	})

	t.Run("descending turns the comparison round", func(t *testing.T) {
		x := require.New(t)

		q, _ := where(t, []sqlpage.Order{{Column: "id", Desc: true}}, []any{7})
		x.Contains(q, "`user`.`id` < ?")
	})

	t.Run("a tiebreaker only applies where the first column ties", func(t *testing.T) {
		x := require.New(t)

		// The whole of keyset paging: rows strictly past the first column, and
		// where it is equal, rows strictly past the tiebreaker. Anything
		// looser repeats a row and anything tighter skips one.
		q, args := where(t,
			[]sqlpage.Order{{Column: "date_created", Desc: true}, {Column: "id", Desc: true}},
			[]any{"t", 7})

		x.Contains(q, "`user`.`date_created` < ?")
		x.Contains(q, "OR")
		x.Contains(q, "`user`.`date_created` = ?")
		x.Contains(q, "`user`.`id` < ?")
		x.Equal([]any{"t", "t", 7}, args)
	})

	t.Run("mixed directions are each their own", func(t *testing.T) {
		x := require.New(t)

		q, _ := where(t,
			[]sqlpage.Order{{Column: "name"}, {Column: "id", Desc: true}},
			[]any{"a", 7})
		x.Contains(q, "`user`.`name` > ?")
		x.Contains(q, "`user`.`id` < ?")
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
