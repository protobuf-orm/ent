// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package keyset holds what a real engine says about a page read after a
// cursor (sqlpage.After): that every row comes back once, in the order asked
// for, across pages that end inside a run of equal values, and -- on
// PostgreSQL, where the predicate's shape decides it -- that the read of a
// page starts at the cursor rather than at the first row.
//
// Driver-level, as enttime is: what is under test is the predicate, and a
// generated client would put a second thing between it and the plan.
package keyset

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/dialect/sql/sqlpage"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestSQLite(t *testing.T) {
	drv, err := sql.Open(dialect.SQLite, "file:"+t.TempDir()+"/db.sqlite")
	require.NoError(t, err)
	defer drv.Close()
	pages(t, drv)
}

func TestMySql(t *testing.T) {
	for version, port := range map[string]int{"8": 3308, "84": 3309} {
		t.Run(version, func(t *testing.T) { onMySql(t, port) })
	}
}

func TestMaria(t *testing.T) {
	for version, port := range map[string]int{"1011": 4309, "114": 4310} {
		t.Run(version, func(t *testing.T) { onMySql(t, port) })
	}
}

func onMySql(t *testing.T, port int) {
	ctx := context.Background()
	root, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/", port))
	require.NoError(t, err)
	defer root.Close()
	require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS keyset", []any{}, nil))
	require.NoError(t, root.Exec(ctx, "CREATE DATABASE keyset", []any{}, nil))
	defer root.Exec(ctx, "DROP DATABASE IF EXISTS keyset", []any{}, nil)

	drv, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/keyset", port))
	require.NoError(t, err)
	defer drv.Close()
	pages(t, drv)
}

func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{"14": 5434, "17": 5437} {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			dsn := fmt.Sprintf("host=localhost port=%d user=postgres password=pass sslmode=disable", port)
			root, err := sql.Open(dialect.Postgres, dsn)
			require.NoError(t, err)
			defer root.Close()
			require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS keyset", []any{}, nil))
			require.NoError(t, root.Exec(ctx, "CREATE DATABASE keyset", []any{}, nil))
			defer root.Exec(ctx, "DROP DATABASE IF EXISTS keyset", []any{}, nil)

			drv, err := sql.Open(dialect.Postgres, dsn+" dbname=keyset")
			require.NoError(t, err)
			defer drv.Close()
			pages(t, drv)
		})
	}
}

// row is one row of the table, as a page reads it.
type row struct{ Id, C1, C2 int }

// size is a page, which is not a multiple of the runs of equal c1 below, so
// that pages end inside them.
const size = 50

// orders are what a page is read in, each with an index to read it by: one
// way up, one way down, and two of mixed directions, the second of which
// leads with two columns that run one way.
var orders = []struct {
	name string
	by   []sqlpage.Order
}{
	{"up", []sqlpage.Order{{Column: "c1"}, {Column: "id"}}},
	{"down", []sqlpage.Order{{Column: "c1", Desc: true}, {Column: "id", Desc: true}}},
	{"mixed", []sqlpage.Order{{Column: "c1", Desc: true}, {Column: "id"}}},
	{"mixed after two", []sqlpage.Order{{Column: "c1"}, {Column: "c2"}, {Column: "id", Desc: true}}},
}

// pages is the same things on every engine.
func pages(t *testing.T, drv *sql.Driver) {
	ctx := context.Background()
	exec := func(query string, args ...any) {
		t.Helper()
		require.NoError(t, drv.Exec(ctx, query, args, new(sql.Result)))
	}

	exec("DROP TABLE IF EXISTS keyset_row")
	exec("CREATE TABLE keyset_row (id integer PRIMARY KEY, tenant integer NOT NULL, c1 integer NOT NULL, c2 integer NOT NULL)")
	defer drv.Exec(ctx, "DROP TABLE IF EXISTS keyset_row", []any{}, new(sql.Result))
	// Tenant first, as a list narrowed to one is read.
	exec("CREATE INDEX keyset_c1 ON keyset_row (tenant, c1, id)")
	exec("CREATE INDEX keyset_c1_c2 ON keyset_row (tenant, c1, c2, id)")

	// Runs of seven rows of one c1, and in the middle one run of 150, which
	// is longer than a page. Every fifth row is another tenant's, so its rows
	// sit between the tenant's in every index.
	var (
		mine []row
		vals []string
		args []any
	)
	for i := range 500 {
		tenant, c1 := 1, i/7
		if i%5 == 4 {
			tenant = 2
		}
		if 200 <= i && i < 350 {
			c1 = 30
		}
		r := row{Id: i + 1, C1: c1, C2: (i * 37) % 11}
		if tenant == 1 {
			mine = append(mine, r)
		}
		vals = append(vals, "(?, ?, ?, ?)")
		args = append(args, r.Id, tenant, r.C1, r.C2)
		if len(vals) == 100 {
			exec(rebind(drv.Dialect(), "INSERT INTO keyset_row (id, tenant, c1, c2) VALUES "+strings.Join(vals, ", ")), args...)
			vals, args = nil, nil
		}
	}
	if drv.Dialect() == dialect.Postgres {
		exec("ANALYZE keyset_row")
	}

	for _, o := range orders {
		t.Run(o.name, func(t *testing.T) {
			want := slices.Clone(mine)
			slices.SortFunc(want, func(a, b row) int {
				for _, by := range o.by {
					if c := cmp.Compare(value(a, by.Column), value(b, by.Column)); c != 0 {
						if by.Desc {
							return -c
						}
						return c
					}
				}
				return 0
			})

			var (
				got  []row
				last *row
			)
			for page := 0; ; page++ {
				q, args := query(t, drv.Dialect(), o.by, last)
				read := scan(t, ctx, drv, q, args)
				if last != nil {
					starts(t, ctx, drv, q, args, ties(mine, o.by, *last))
				}
				got = append(got, read...)
				if len(read) < size {
					break
				}
				last = &read[len(read)-1]
				require.Less(t, page, len(mine)/size+1, "pages without end")
			}
			require.Equal(t, want, got, "every row once, in order")
		})
	}
}

// query is a page of the tenant's rows in an order, after the last row of
// the page before, or the first page when there was none.
func query(t *testing.T, engine string, by []sqlpage.Order, last *row) (string, []any) {
	t.Helper()
	s := sql.Dialect(engine).Select("id", "c1", "c2").From(sql.Table("keyset_row")).Where(sql.EQ("tenant", 1))
	if last != nil {
		at := make([]any, len(by))
		for i, o := range by {
			at[i] = value(*last, o.Column)
		}
		after, err := sqlpage.After(by, at)
		require.NoError(t, err)
		after(s)
	}
	for _, o := range by {
		if o.Desc {
			s.OrderBy(sql.Desc(s.C(o.Column)))
			continue
		}
		s.OrderBy(s.C(o.Column))
	}
	return s.Limit(size).Query()
}

func scan(t *testing.T, ctx context.Context, drv *sql.Driver, q string, args []any) []row {
	t.Helper()
	rows := &sql.Rows{}
	require.NoError(t, drv.Query(ctx, q, args, rows))
	defer rows.Close()
	var out []row
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.Id, &r.C1, &r.C2))
		out = append(out, r)
	}
	require.NoError(t, rows.Err())
	return out
}

// ties is how many of the rows are equal to the cursor in the columns that
// lead the order one way, which is what a bound on them cannot skip.
func ties(rows []row, by []sqlpage.Order, at row) int {
	run := 1
	for run < len(by) && by[run].Desc == by[0].Desc {
		run++
	}
	n := 0
	for _, r := range rows {
		if !slices.ContainsFunc(by[:run], func(o sqlpage.Order) bool { return value(r, o.Column) != value(at, o.Column) }) {
			n++
		}
	}
	return n
}

// starts is whether the read of a page began at the cursor.
//
// On PostgreSQL that is the plan's to say, and it is why the predicate has
// the shape it has: what an index scan threw away is what it read before
// reaching the cursor, and the bound may only cost the rows tied with the
// cursor in the columns it covers. The OR alone is the rows before the cursor
// -- a page's worth on the second page, and more on each after it. Sequential
// and bitmap scans are off, so that a table this small is planned as a large
// one is.
//
// On SQLite the plan names the columns the index search is constrained by,
// and the order's first is among them. Given the OR alone it searched by the
// tenant only.
//
// MySQL and MariaDB read the OR as ranges of the index, and nothing here
// changes what they are given beyond a bound the ranges already imply.
func starts(t *testing.T, ctx context.Context, drv *sql.Driver, q string, args []any, ties int) {
	t.Helper()
	switch drv.Dialect() {
	case dialect.Postgres:
		tx, err := drv.Tx(ctx)
		require.NoError(t, err)
		defer tx.Rollback()
		for _, set := range []string{"SET LOCAL enable_seqscan = off", "SET LOCAL enable_bitmapscan = off"} {
			require.NoError(t, tx.Exec(ctx, set, []any{}, new(sql.Result)))
		}
		rows := &sql.Rows{}
		require.NoError(t, tx.Query(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+q, args, rows))
		var plan []byte
		require.True(t, rows.Next())
		require.NoError(t, rows.Scan(&plan))
		require.NoError(t, rows.Close())

		var root []struct{ Plan node }
		require.NoError(t, json.Unmarshal(plan, &root))
		scans := root[0].Plan.scans()
		require.Len(t, scans, 1, "%s", plan)
		s := scans[0]
		require.Contains(t, s.Type, "Index", "%s", plan)
		require.Contains(t, s.Cond, "c1", "the index is not read from the cursor: %s", plan)
		require.LessOrEqual(t, s.Removed, ties,
			"a page read %d rows by %s %s (%s) and threw %d of them away, and %d rows are tied with the cursor",
			s.Rows+s.Removed, s.Type, s.Index, s.Cond, s.Removed, ties)

	case dialect.SQLite:
		rows := &sql.Rows{}
		require.NoError(t, drv.Query(ctx, "EXPLAIN QUERY PLAN "+q, args, rows))
		defer rows.Close()
		var details []string
		for rows.Next() {
			var (
				id, parent, unused int
				detail             string
			)
			require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
			details = append(details, detail)
		}
		require.NoError(t, rows.Err())
		i := slices.IndexFunc(details, func(d string) bool { return strings.HasPrefix(d, "SEARCH keyset_row") })
		require.NotEqual(t, -1, i, "%q", details)
		_, cond, _ := strings.Cut(details[i], "(")
		require.Contains(t, cond, "c1", "the index is not read from the cursor: %q", details)
	}
}

// node is a node of PostgreSQL's plan, as much of it as is read here.
type node struct {
	Type    string `json:"Node Type"`
	Index   string `json:"Index Name"`
	Cond    string `json:"Index Cond"`
	Rows    int    `json:"Actual Rows"`
	Removed int    `json:"Rows Removed by Filter"`
	Plans   []node `json:"Plans"`
}

// scans are the nodes of the plan that read the table.
func (n node) scans() []node {
	if strings.HasSuffix(n.Type, "Scan") {
		return []node{n}
	}
	var out []node
	for _, c := range n.Plans {
		out = append(out, c.scans()...)
	}
	return out
}

func value(r row, column string) int {
	switch column {
	case "id":
		return r.Id
	case "c1":
		return r.C1
	case "c2":
		return r.C2
	}
	panic(column)
}

// rebind numbers the placeholders, which PostgreSQL wants and the others do
// not take.
func rebind(engine, q string) string {
	if engine != dialect.Postgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
