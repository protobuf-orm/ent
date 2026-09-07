// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package enttime holds what a real engine says about the times ent writes and
// reads back.
//
// It is driver-level on purpose. What is under test is dialect/sql -- the
// argument it binds and the value it scans -- and a generated client would put
// a second thing between the assertion and the answer.
package enttime

import (
	"context"
	dbsql "database/sql"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/entc/integration/ent"
	"github.com/protobuf-orm/ent/entc/integration/ent/enttest"
	"github.com/protobuf-orm/ent/entc/integration/ent/license"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

// at is one instant. kst and hst are two ways of writing it that a deployment
// produces without anybody choosing between them: a laptop that has a zone, a
// container that has none.
var (
	at  = time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	kst = time.FixedZone("KST", 9*60*60)
	hst = time.FixedZone("HST", -9*60*60)
)

func TestSQLite(t *testing.T) {
	dsn := "file:" + t.TempDir() + "/db.sqlite"
	drv, err := sql.Open(dialect.SQLite, dsn)
	require.NoError(t, err)
	defer drv.Close()
	compares(t, drv, "datetime")

	// A row SQLite holds with an offset on it, which is what a row written
	// before this package moved times to UTC looks like, and what a row
	// written by anything that is not ent looks like now.
	t.Run("reads UTC", func(t *testing.T) {
		ctx := context.Background()
		require.NoError(t, drv.Exec(ctx, "CREATE TABLE enttime_read (at datetime NOT NULL)", []any{}, new(sql.Result)))
		db, err := dbsql.Open("sqlite3", dsn)
		require.NoError(t, err)
		defer db.Close()
		_, err = db.ExecContext(ctx, "INSERT INTO enttime_read (at) VALUES (?)", at.In(kst))
		require.NoError(t, err)
		readsUTC(t, ctx, drv, "SELECT at FROM enttime_read")
	})
}

func TestMySql(t *testing.T) {
	for version, port := range map[string]int{"8": 3308, "84": 3309} {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			root, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/", port))
			require.NoError(t, err)
			defer root.Close()
			require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS enttime", []any{}, new(sql.Result)))
			require.NoError(t, root.Exec(ctx, "CREATE DATABASE enttime", []any{}, new(sql.Result)))
			defer root.Exec(ctx, "DROP DATABASE IF EXISTS enttime", []any{}, new(sql.Result))

			drv, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/enttime?parseTime=True", port))
			require.NoError(t, err)
			defer drv.Close()

			// Both of MySql's time columns, because they differ in a way that
			// matters here: a timestamp is converted through the session's
			// time_zone and a datetime is not.
			for _, col := range []string{"datetime", "timestamp"} {
				t.Run(col, func(t *testing.T) { compares(t, drv, col) })
			}

			// A connection that reads in a zone of its own, which is what a
			// DSN with `loc` set asks for. The column still answers in UTC.
			t.Run("reads UTC", func(t *testing.T) {
				local, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/enttime?parseTime=True&loc=Asia%%2FSeoul", port))
				require.NoError(t, err)
				defer local.Close()
				require.NoError(t, local.Exec(ctx, "DROP TABLE IF EXISTS enttime_read", []any{}, new(sql.Result)))
				require.NoError(t, local.Exec(ctx, "CREATE TABLE enttime_read (at datetime NOT NULL)", []any{}, new(sql.Result)))
				defer local.Exec(ctx, "DROP TABLE IF EXISTS enttime_read", []any{}, new(sql.Result))
				require.NoError(t, local.Exec(ctx, "INSERT INTO enttime_read (at) VALUES (?)", []any{at}, new(sql.Result)))
				readsUTC(t, ctx, local, "SELECT at FROM enttime_read")
			})

			// What column a time field actually gets, and what that buys.
			//
			// A MySql timestamp is not a column so much as a conversion: what
			// is written to one is read in the session's `time_zone` and
			// stored as UTC, and converted back on the way out. The setting
			// is not in any schema, defaults to the server's own zone, and
			// two connections need not share it -- so the same row read on
			// two connections is two instants, and ent's UTC is taken for
			// whatever the session says. A datetime stores what it is given.
			t.Run("the session's time_zone is not the column's", func(t *testing.T) {
				client := ent.NewClient(ent.Driver(drv))
				require.NoError(t, client.Schema.Create(ctx))

				rows := &sql.Rows{}
				require.NoError(t, drv.Query(ctx,
					"SELECT data_type FROM information_schema.columns WHERE table_schema = 'enttime' AND table_name = ? AND column_name = ?",
					[]any{license.Table, license.FieldCreateTime}, rows))
				typ, err := sql.ScanString(rows)
				require.NoError(t, err)
				require.NoError(t, rows.Close())
				require.Equal(t, "datetime", typ)

				// Written on a connection whose session sits in one zone and
				// read on one in another. On a timestamp the two answers
				// would be eighteen hours apart.
				//
				// The zone is set in the DSN rather than with sql.WithVar
				// because that resets a MySql variable by setting it to NULL,
				// which `time_zone` refuses.
				zoned := func(zone string) *ent.Client {
					t.Helper()
					d, err := sql.Open(dialect.MySql, fmt.Sprintf(
						"root:pass@tcp(localhost:%d)/enttime?parseTime=True&time_zone=%s",
						port, url.QueryEscape("'"+zone+"'"),
					))
					require.NoError(t, err)
					t.Cleanup(func() { d.Close() })
					return ent.NewClient(ent.Driver(d))
				}
				l := zoned("+09:00").License.Create().SetId(1).SetCreateTime(at).SetUpdateTime(at).SaveX(ctx)
				require.Equal(t, at, zoned("-09:00").License.GetX(ctx, l.Id).CreateTime)
			})
		})
	}
}

func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{"14": 5434, "17": 5437} {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			dsn := fmt.Sprintf("host=localhost port=%d user=postgres password=pass sslmode=disable", port)
			root, err := sql.Open(dialect.Postgres, dsn)
			require.NoError(t, err)
			defer root.Close()
			require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS enttime", []any{}, nil))
			require.NoError(t, root.Exec(ctx, "CREATE DATABASE enttime", []any{}, nil))
			defer root.Exec(ctx, "DROP DATABASE IF EXISTS enttime", []any{}, nil)

			drv, err := sql.Open(dialect.Postgres, dsn+" dbname=enttime")
			require.NoError(t, err)
			defer drv.Close()
			for _, col := range []string{"timestamptz", "timestamp"} {
				t.Run(col, func(t *testing.T) { compares(t, drv, col) })
			}

			// A session in a zone of its own, which is what a Postgres whose
			// TimeZone was never set to UTC gives every connection. A
			// timestamptz comes back with that offset on it; the column still
			// answers in UTC.
			t.Run("reads UTC", func(t *testing.T) {
				ctx := sql.WithVar(ctx, "TimeZone", "Asia/Seoul")
				require.NoError(t, drv.Exec(ctx, "DROP TABLE IF EXISTS enttime_read", []any{}, new(sql.Result)))
				require.NoError(t, drv.Exec(ctx, "CREATE TABLE enttime_read (at timestamptz NOT NULL)", []any{}, new(sql.Result)))
				defer drv.Exec(context.Background(), "DROP TABLE IF EXISTS enttime_read", []any{}, new(sql.Result))
				require.NoError(t, drv.Exec(ctx, "INSERT INTO enttime_read (at) VALUES ($1)", []any{at}, new(sql.Result)))
				readsUTC(t, ctx, drv, "SELECT at FROM enttime_read")
			})
		})
	}
}

// readsUTC is what dialect/sql promises about a time it scans: the instant the
// column holds, in one zone, whatever the engine answered in.
//
// Each caller arranges for the engine to answer in something else -- a session
// TimeZone, a connection `loc`, an offset already written into the text -- so
// that a plain pass-through would be visible here.
func readsUTC(t *testing.T, ctx context.Context, drv *sql.Driver, query string) {
	t.Helper()
	rows := &sql.Rows{}
	require.NoError(t, drv.Query(ctx, query, []any{}, rows))
	defer rows.Close()
	require.True(t, rows.Next())

	var got time.Time
	require.NoError(t, rows.Scan(&got))
	require.Equal(t, time.UTC, got.Location(), "scanned %s", got)
	require.True(t, at.Equal(got), "scanned %s, want %s", got, at)

	// Which is the point: the value compares equal to a written-down one, and
	// not merely to itself.
	require.Equal(t, at, got)
}

// compares is the whole claim: rows written from different zones are one value
// to the engine, so `=`, `>` and ORDER BY answer about the instant.
//
// SQLite is where this can fail and be believed. Its datetime is text, its
// default collation is byte order, and RFC 3339 with an offset does not sort
// -- so one instant written from two zones is two keys, and a cursor built on
// `>` (see dialect/sql/sqlpage) walks past rows or repeats them.
//
// The other two were never wrong here, and are checked anyway. go-sql-driver
// renders a time in the connection's own location before sending it and lib/pq
// sends the offset for Postgres to parse, so both had an answer already. What
// is under test there is that ent's answer is the same one, on every column
// type an instant can land in.
func compares(t *testing.T, drv *sql.Driver, col string) {
	ctx := context.Background()
	// Queries are written with `?` and rewritten for the one dialect that
	// numbers its placeholders. Building them with sql.Dialect would say the
	// same thing at four times the length, and hide the SQL being asserted on.
	bind := func(query string) string {
		if drv.Dialect() != dialect.Postgres {
			return query
		}
		for i := 1; strings.Contains(query, "?"); i++ {
			query = strings.Replace(query, "?", fmt.Sprintf("$%d", i), 1)
		}
		return query
	}
	exec := func(query string, args ...any) {
		t.Helper()
		require.NoError(t, drv.Exec(ctx, bind(query), args, new(sql.Result)))
	}
	exec("DROP TABLE IF EXISTS enttime")
	exec("CREATE TABLE enttime (id integer NOT NULL, at " + col + " NOT NULL)")
	defer exec("DROP TABLE IF EXISTS enttime")

	// The first three are one instant; the fourth is an hour after it.
	exec("INSERT INTO enttime (id, at) VALUES (?, ?)", 1, at.In(kst))
	exec("INSERT INTO enttime (id, at) VALUES (?, ?)", 2, at)
	exec("INSERT INTO enttime (id, at) VALUES (?, ?)", 3, at.In(hst))
	exec("INSERT INTO enttime (id, at) VALUES (?, ?)", 4, at.Add(time.Hour))

	ids := func(query string, args ...any) []int {
		t.Helper()
		rows := &sql.Rows{}
		require.NoError(t, drv.Query(ctx, bind(query), args, rows))
		defer rows.Close()
		var out []int
		require.NoError(t, sql.ScanSlice(rows, &out))
		return out
	}

	// Ordering is by instant, so the three that share one come before the row
	// an hour later -- in any order among themselves, which is why the query
	// breaks the tie by id.
	require.Equal(t, []int{1, 2, 3, 4}, ids("SELECT id FROM enttime ORDER BY at, id"))

	// Equality finds every row holding the instant, not the one that happens
	// to have been written the same way as the argument.
	require.Equal(t, []int{1, 2, 3}, ids("SELECT id FROM enttime WHERE at = ? ORDER BY id", at))

	// And a cursor past that instant finds what comes after it: the row an
	// hour later, and nothing else.
	require.Equal(t, []int{4}, ids("SELECT id FROM enttime WHERE at > ? ORDER BY id", at))

	// The zone the argument was written in makes no difference to any of it.
	require.Equal(t, []int{1, 2, 3}, ids("SELECT id FROM enttime WHERE at = ? ORDER BY id", at.In(kst)))
	require.Equal(t, []int{4}, ids("SELECT id FROM enttime WHERE at > ? ORDER BY id", at.In(hst)))
}

// TestGeneratedClient is the same claim seen from where an app stands: what a
// Create returns and what a Get reads back are one value, in one zone.
//
// It is worth asserting through generated code because the two halves are
// reached differently. The default that fills create_time is the schema
// descriptor's, which a generated runtime.go asks for at init; the value the
// node carries never goes near a driver, since a Create builds its node from
// the mutation rather than reading the row back.
func TestGeneratedClient(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:enttime?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()

	l := client.License.Create().SetId(1).SaveX(ctx)
	require.Equal(t, time.UTC, l.CreateTime.Location(), "create_time is %s", l.CreateTime)
	require.Equal(t, time.UTC, l.UpdateTime.Location(), "update_time is %s", l.UpdateTime)

	// Not merely the same instant: the same value, which is what a test that
	// compares a response to a row gets to say.
	got := client.License.GetX(ctx, l.Id)
	require.Equal(t, l.CreateTime, got.CreateTime)
	require.Equal(t, l.UpdateTime, got.UpdateTime)

	// An update takes the same route through UpdateDefault.
	u := client.License.UpdateOne(l).SaveX(ctx)
	require.Equal(t, time.UTC, u.UpdateTime.Location(), "update_time is %s", u.UpdateTime)
	require.Equal(t, u.UpdateTime, client.License.GetX(ctx, l.Id).UpdateTime)
}
