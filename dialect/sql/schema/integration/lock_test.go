// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package integration

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/protobuf-orm/ent/dialect"
	entschema "github.com/protobuf-orm/ent/dialect/sql/schema"
	"github.com/stretchr/testify/require"
)

// Postgres names a PostgreSQL server the tests that need one may make a
// database on, and drop it again:
//
//	ENT_SCHEMA_POSTGRES='postgres://postgres:pass@127.0.0.1:5432/test?sslmode=disable' go test ./...
//
// Everything else here runs on SQLite. What needs a server is what is only
// true of one: several sessions at once.
const Postgres = "ENT_SCHEMA_POSTGRES"

// postgres is a database of its own on the server [Postgres] names, dropped
// when the test ends: what reaches it, as a connection string.
func postgres(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv(Postgres)
	if dsn == "" {
		t.Skipf("%s is not set; see the const for what it wants", Postgres)
	}

	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close() })

	name := fmt.Sprintf("ent_schema_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE "+name)
	require.NoError(t, err)
	// Last, after every pool the test opened on it is closed, or it is in use.
	t.Cleanup(func() {
		_, err := admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		require.NoError(t, err)
	})

	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + name

	return u.String()
}

// pool is a pool of connections to `dsn`, closed when the test ends.
func pool(t *testing.T, dsn string) *sql.DB {
	t.Helper()

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	return db
}

// TestApplyIsOneAtATime: a deployment applies on every start, and so does
// each replica of it. Two that ran one file at once would both run its
// statements, and the second to reach one that cannot run twice would fail --
// and record the file as failed over the record of the one that ran it, so
// that every start after runs it again and fails again. So a database is
// migrated by one of them at a time, and the other finds nothing left to do.
func TestApplyIsOneAtATime(t *testing.T) {
	ctx := t.Context()

	dir, err := entschema.OpenDir(t.TempDir())
	require.NoError(t, err)
	m := entschema.Migrations{Dir: dir, Dialect: dialect.Postgres}

	dsn := postgres(t)
	db := pool(t, dsn)

	// Both at once, as two processes: two pools, so nothing either holds is
	// the other's.
	both := func(t *testing.T) {
		t.Helper()

		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, db := range []*sql.DB{db, pool(t, dsn)} {
			wg.Go(func() {
				_, errs[i] = m.Apply(ctx, db, dialect.Postgres)
			})
		}
		wg.Wait()

		require.NoError(t, errs[0])
		require.NoError(t, errs[1])
	}

	// What the history says of a version: how far it ran, and what stopped it.
	ran := func(t *testing.T, version string) (int, int, string) {
		t.Helper()

		var applied, total int
		var failed string
		require.NoError(t, db.QueryRowContext(ctx,
			`SELECT applied, total, error FROM `+entschema.RevisionTable+` WHERE version = $1`, version).
			Scan(&applied, &total, &failed))

		return applied, total, failed
	}

	t.Run("the first, which makes the history as well", func(t *testing.T) {
		x := require.New(t)

		_, err := m.Write("20260101000000", "first", []byte("CREATE TABLE first (id integer NOT NULL);\n"))
		x.NoError(err)

		both(t)

		applied, total, failed := ran(t, "20260101000000")
		x.Equal(1, total)
		x.Equal(total, applied)
		x.Empty(failed)
	})

	t.Run("and one both find pending", func(t *testing.T) {
		x := require.New(t)

		// Slow enough that the two are in it together, and with a statement
		// that cannot be run twice.
		_, err := m.Write("20260102000000", "once", []byte("SELECT pg_sleep(0.5);\nCREATE TABLE once (id integer NOT NULL);\n"))
		x.NoError(err)

		both(t)

		applied, total, failed := ran(t, "20260102000000")
		x.Equal(2, total)
		x.Equal(total, applied, "the history says the file was not run to its end")
		x.Empty(failed)

		fs, err := m.Pending(ctx, db, dialect.Postgres)
		x.NoError(err)
		x.Empty(fs)
	})
}
