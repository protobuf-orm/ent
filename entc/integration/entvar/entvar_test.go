// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package entvar holds what a real engine says about sql.WithVar.
//
// Setting a session variable is the easy half. Putting it back is where the
// engines differ, and where a wrong answer is invisible in a unit test: the
// reset runs after the query, on the way back to the pool, so a statement that
// succeeded reports an error from a place the caller never named.
package entvar

import (
	"context"
	"fmt"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestMySql(t *testing.T) {
	for version, port := range map[string]int{"8": 3308, "84": 3309} {
		t.Run(version, func(t *testing.T) {
			drv, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/test", port))
			require.NoError(t, err)
			defer drv.Close()
			mysqlVars(t, drv)
		})
	}
}

func TestMaria(t *testing.T) {
	for version, port := range map[string]int{"1011": 4309, "114": 4310} {
		t.Run(version, func(t *testing.T) {
			drv, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/test", port))
			require.NoError(t, err)
			defer drv.Close()
			mysqlVars(t, drv)
		})
	}
}

// mysqlVars covers both kinds of variable, because each refuses the other's
// way of being unset. A system variable goes back with DEFAULT and rejects
// NULL outright -- `SET time_zone = NULL` is error 1231; a user-defined
// variable has no global value to go back to, takes NULL, and answers DEFAULT
// with a syntax error.
//
// The variable is one whose value can be read back, so that the test says the
// session was really left as it was found rather than that no error came out.
func mysqlVars(t *testing.T, drv *sql.Driver) {
	ctx := context.Background()
	read := func(ctx context.Context, expr string) string {
		t.Helper()
		rows := &sql.Rows{}
		require.NoError(t, drv.Query(ctx, "SELECT "+expr, []any{}, rows))
		defer rows.Close()
		v, err := sql.ScanString(rows)
		require.NoError(t, err)
		return v
	}

	t.Run("a system variable", func(t *testing.T) {
		before := read(ctx, "@@SESSION.time_zone")
		require.NotEqual(t, "+09:00", before, "the server is already in the zone this asks for")

		// The query runs with the variable set, and the connection it ran on
		// goes back to the pool without it.
		require.Equal(t, "+09:00", read(sql.WithVar(ctx, "time_zone", "+09:00"), "@@SESSION.time_zone"))
		require.Equal(t, before, read(ctx, "@@SESSION.time_zone"))
	})

	t.Run("a variable of one's own", func(t *testing.T) {
		require.Equal(t, "hello", read(sql.WithVar(ctx, "@ent", "hello"), "@ent"))

		// Unset, which for this kind of variable is what NULL means. A NULL
		// scans to the empty string here, and the point is that it is no
		// longer "hello".
		rows := &sql.Rows{}
		require.NoError(t, drv.Query(ctx, "SELECT COALESCE(@ent, '')", []any{}, rows))
		defer rows.Close()
		v, err := sql.ScanString(rows)
		require.NoError(t, err)
		require.Empty(t, v)
	})
}

// TestPostgres is the other dialect that has a reset at all, and it has one
// statement for every variable: RESET.
func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{"14": 5434, "17": 5437} {
		t.Run(version, func(t *testing.T) {
			drv, err := sql.Open(dialect.Postgres, fmt.Sprintf("host=localhost port=%d user=postgres password=pass dbname=test sslmode=disable", port))
			require.NoError(t, err)
			defer drv.Close()
			ctx := context.Background()
			read := func(ctx context.Context) string {
				t.Helper()
				rows := &sql.Rows{}
				require.NoError(t, drv.Query(ctx, "SHOW TimeZone", []any{}, rows))
				defer rows.Close()
				v, err := sql.ScanString(rows)
				require.NoError(t, err)
				return v
			}
			before := read(ctx)
			require.NotEqual(t, "Asia/Seoul", before, "the server is already in the zone this asks for")
			require.Equal(t, "Asia/Seoul", read(sql.WithVar(ctx, "TimeZone", "Asia/Seoul")))
			require.Equal(t, before, read(ctx))
		})
	}
}
