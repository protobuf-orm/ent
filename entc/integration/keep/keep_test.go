// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package keep holds what a real engine says about the work a transaction
// keeps for its commit: that it is done inside the transaction, before the
// commit, and holds or falls with it.
//
// Driver-level, as enttime is: what is under test is dialect.BeginTx and the
// transaction database/sql hands it, and a generated client would put a second
// thing between the assertion and the answer.
package keep

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestSQLite(t *testing.T) {
	drv, err := sql.Open(dialect.SQLite, "file:"+t.TempDir()+"/db.sqlite")
	require.NoError(t, err)
	defer drv.Close()
	keeps(t, drv)
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
	require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS keep", []any{}, new(sql.Result)))
	require.NoError(t, root.Exec(ctx, "CREATE DATABASE keep", []any{}, new(sql.Result)))
	defer root.Exec(ctx, "DROP DATABASE IF EXISTS keep", []any{}, new(sql.Result))

	drv, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/keep", port))
	require.NoError(t, err)
	defer drv.Close()
	keeps(t, drv)
}

func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{"14": 5434, "17": 5437} {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			dsn := fmt.Sprintf("host=localhost port=%d user=postgres password=pass sslmode=disable", port)
			root, err := sql.Open(dialect.Postgres, dsn)
			require.NoError(t, err)
			defer root.Close()
			require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS keep", []any{}, nil))
			require.NoError(t, root.Exec(ctx, "CREATE DATABASE keep", []any{}, nil))
			defer root.Exec(ctx, "DROP DATABASE IF EXISTS keep", []any{}, nil)

			drv, err := sql.Open(dialect.Postgres, dsn+" dbname=keep")
			require.NoError(t, err)
			defer drv.Close()
			keeps(t, drv)
		})
	}
}

// keeps is the same four things on every engine. Each statement is written
// without arguments, so that one spelling of it is every engine's.
func keeps(t *testing.T, drv *sql.Driver) {
	ctx := context.Background()
	exec := func(d dialect.Driver, query string) error {
		return d.Exec(ctx, query, []any{}, new(sql.Result))
	}
	count := func(t *testing.T) int {
		t.Helper()
		rows := &sql.Rows{}
		require.NoError(t, drv.Query(ctx, "SELECT COUNT(*) FROM keep_row", []any{}, rows))
		defer rows.Close()
		require.True(t, rows.Next())
		var n int
		require.NoError(t, rows.Scan(&n))
		return n
	}

	require.NoError(t, exec(drv, "DROP TABLE IF EXISTS keep_row"))
	require.NoError(t, exec(drv, "CREATE TABLE keep_row (n integer NOT NULL)"))
	defer exec(drv, "DROP TABLE IF EXISTS keep_row")
	empty := func(t *testing.T) {
		t.Helper()
		require.NoError(t, exec(drv, "DELETE FROM keep_row"))
	}

	t.Run("the kept work is done in the transaction and holds with it", func(t *testing.T) {
		empty(t)
		d, tx, err := dialect.BeginTx(ctx, drv)
		require.NoError(t, err)
		defer tx.Rollback()

		k, ok := dialect.KeepOf(d)
		require.True(t, ok)
		require.NoError(t, exec(d, "INSERT INTO keep_row (n) VALUES (1)"))
		k.BeforeCommit(func(context.Context) error { return exec(d, "INSERT INTO keep_row (n) VALUES (2)") })

		require.NoError(t, tx.Commit())
		require.Equal(t, 2, count(t))
	})

	t.Run("a rollback takes it along unrun", func(t *testing.T) {
		empty(t)
		d, tx, err := dialect.BeginTx(ctx, drv)
		require.NoError(t, err)

		k, _ := dialect.KeepOf(d)
		k.BeforeCommit(func(context.Context) error { return exec(d, "INSERT INTO keep_row (n) VALUES (1)") })

		require.NoError(t, tx.Rollback())
		require.Equal(t, 0, count(t))
	})

	t.Run("a refusal from it takes the transaction back", func(t *testing.T) {
		empty(t)
		d, tx, err := dialect.BeginTx(ctx, drv)
		require.NoError(t, err)
		defer tx.Rollback()

		k, _ := dialect.KeepOf(d)
		require.NoError(t, exec(d, "INSERT INTO keep_row (n) VALUES (1)"))
		refused := errors.New("refused")
		k.BeforeCommit(func(context.Context) error { return refused })

		require.ErrorIs(t, tx.Commit(), refused)
		require.Equal(t, 0, count(t), "a write before a refused commit held")
	})

	t.Run("work kept inside is done by the commit around it", func(t *testing.T) {
		empty(t)
		d, tx, err := dialect.BeginTx(ctx, drv)
		require.NoError(t, err)
		defer tx.Rollback()

		inner, innerTx, err := dialect.BeginTx(ctx, d)
		require.NoError(t, err)
		k, _ := dialect.KeepOf(inner)
		k.BeforeCommit(func(context.Context) error { return exec(inner, "INSERT INTO keep_row (n) VALUES (1)") })
		require.NoError(t, innerTx.Commit())

		require.NoError(t, tx.Commit())
		require.Equal(t, 1, count(t))
	})
}
