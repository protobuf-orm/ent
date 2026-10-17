// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package returning is the one package generated with `sql/returning` on, and
// it holds what each engine says about a create builder's Stored: that it is
// the row a query would read, and that where the dialect has RETURNING it
// costs the INSERT nothing more.
package returning

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/entc/integration/returning/ent"
	"github.com/protobuf-orm/ent/entc/integration/returning/ent/item"
	"github.com/protobuf-orm/ent/entc/integration/returning/ent/thing"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestSQLite(t *testing.T) {
	drv, err := sql.Open(dialect.SQLite, "file:"+t.TempDir()+"/db.sqlite?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer drv.Close()
	reads(t, drv, true)
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

// onMySql is both of the engines that speak MySQL's dialect, and neither has
// an INSERT that answers with what it wrote -- MariaDB's does, but the dialect
// is MySQL's, and that is what the INSERT is written in.
func onMySql(t *testing.T, port int) {
	ctx := context.Background()
	root, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/", port))
	require.NoError(t, err)
	defer root.Close()
	require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS returned", []any{}, nil))
	require.NoError(t, root.Exec(ctx, "CREATE DATABASE returned", []any{}, nil))
	defer root.Exec(ctx, "DROP DATABASE IF EXISTS returned", []any{}, nil)

	drv, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/returned?parseTime=True", port))
	require.NoError(t, err)
	defer drv.Close()
	reads(t, drv, false)
}

func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{"14": 5434, "17": 5437} {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			dsn := fmt.Sprintf("host=localhost port=%d user=postgres password=pass sslmode=disable", port)
			root, err := sql.Open(dialect.Postgres, dsn)
			require.NoError(t, err)
			defer root.Close()
			require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS returned", []any{}, nil))
			require.NoError(t, root.Exec(ctx, "CREATE DATABASE returned", []any{}, nil))
			defer root.Exec(ctx, "DROP DATABASE IF EXISTS returned", []any{}, nil)

			drv, err := sql.Open(dialect.Postgres, dsn+" dbname=returned")
			require.NoError(t, err)
			defer drv.Close()
			reads(t, drv, true)

			// The one difference every engine but SQLite makes and this one
			// makes finer than MySQL: a time is kept to the microsecond.
			// Stored says so where Save does not.
			client := ent.NewClient(ent.Driver(drv))
			var stored *ent.Thing
			made := client.Thing.Create().SetName("precise").SetAt(at).Stored(&stored).SaveX(ctx)
			require.Equal(t, at, made.At)
			require.Equal(t, at.Truncate(time.Microsecond), stored.At.UTC())
		})
	}
}

// at has nanoseconds on purpose: only SQLite keeps all of them.
var at = time.Date(2026, 10, 18, 12, 34, 56, 789123456, time.UTC)

// reads is the same things on every engine. `returning` is whether the
// dialect's INSERT answers with the row, which is what decides how many
// statements reading it takes.
func reads(t *testing.T, drv *sql.Driver, returning bool) {
	ctx := context.Background()
	var n atomic.Int64
	client := ent.NewClient(ent.Driver(dialect.DebugWithContext(drv, func(context.Context, ...any) {
		n.Add(1)
	})))
	require.NoError(t, client.Schema.Create(ctx))

	t.Run("it is the row a query reads, and not the one the builder had", func(t *testing.T) {
		var stored *ent.Thing
		made, err := client.Thing.Create().
			SetName("read").
			SetAt(at).
			SetDoc(json.RawMessage(`{"b": 1,  "a": 2}`)).
			Stored(&stored).
			Save(ctx)
		require.NoError(t, err)
		require.NotNil(t, stored)

		// The default is the column's, so the builder never knew it.
		require.Empty(t, made.Motto)
		require.Equal(t, "cave", stored.Motto)

		got := client.Thing.GetX(ctx, made.Id)
		require.Equal(t, got.Id, stored.Id)
		require.Equal(t, got.Name, stored.Name)
		require.True(t, got.At.Equal(stored.At), "a query read %s, and Stored %s", got.At, stored.At)
		require.Equal(t, got.Motto, stored.Motto)
		// As the database keeps it, which is not how it was written on an
		// engine with a JSON type of its own.
		require.Equal(t, string(got.Doc), string(stored.Doc))
	})

	t.Run("one statement where the INSERT can answer with it", func(t *testing.T) {
		n.Store(0)
		var stored *ent.Thing
		client.Thing.Create().SetName("counted").SetAt(at).Stored(&stored).SaveX(ctx)
		require.NotNil(t, stored)

		want := int64(1)
		if !returning {
			want = 2
		}
		require.Equal(t, want, n.Load())
	})

	t.Run("and nothing read when nobody asked", func(t *testing.T) {
		n.Store(0)
		client.Thing.Create().SetName("unasked").SetAt(at).SaveX(ctx)
		require.Equal(t, int64(1), n.Load())
	})

	// Read on the driver the INSERT ran on: a transaction's row, which is
	// gone with it.
	t.Run("inside a transaction it is that transaction's row", func(t *testing.T) {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)

		var stored *ent.Thing
		made := tx.Thing.Create().SetName("rolled back").SetAt(at).Stored(&stored).SaveX(ctx)
		require.NotNil(t, stored)
		require.Equal(t, "cave", stored.Motto)
		require.NoError(t, tx.Rollback())

		require.Zero(t, client.Thing.Query().Where(thing.Id(made.Id)).CountX(ctx))
	})

	t.Run("a create that fails hands nothing over", func(t *testing.T) {
		stored := &ent.Thing{Name: "as it was"}
		v := stored
		// The same name twice, which the column refuses: the INSERT itself
		// fails, and there is no row to be read.
		client.Thing.Create().SetName("once").SetAt(at).SaveX(ctx)
		_, err := client.Thing.Create().SetName("once").SetAt(at).Stored(&v).Save(ctx)
		require.Error(t, err)
		require.Same(t, stored, v)
	})

	// A batch is one INSERT for all of its rows, and reads every one of them
	// back with it: an Item by the Id the caller made up, whatever order the
	// INSERT answers in, and a Thing by its place, since the database made
	// its Id up.
	t.Run("a batch reads back every row, one statement where it can", func(t *testing.T) {
		n.Store(0)
		// No row sets the motto, so the column is left out of the INSERT and
		// the database's default is every row's. A column some rows of a
		// batch set is NULL for the others, as a batch always was: the
		// default is for a column the INSERT does not name.
		stored := make([]*ent.Item, 3)
		made, err := client.Item.CreateBulk(
			client.Item.Create().SetName("first").Stored(&stored[0]),
			client.Item.Create().SetName("second").Stored(&stored[1]),
			client.Item.Create().SetName("third").Stored(&stored[2]),
		).Save(ctx)
		require.NoError(t, err)

		want := int64(1)
		if !returning {
			want = 2
		}
		require.Equal(t, want, n.Load())

		for i, v := range stored {
			require.NotNil(t, v, "item %d", i)
			require.Equal(t, made[i].Id, v.Id, "item %d was handed another's row", i)
			require.Equal(t, made[i].Name, v.Name, "item %d was handed another's row", i)
			got := client.Item.GetX(ctx, v.Id)
			require.Equal(t, got.Name, v.Name)
			require.Equal(t, got.Motto, v.Motto)
			require.Empty(t, made[i].Motto)
			require.Equal(t, "cave", v.Motto)
		}
	})

	t.Run("and a batch the database makes the Ids of, by place", func(t *testing.T) {
		stored := make([]*ent.Thing, 2)
		made, err := client.Thing.CreateBulk(
			client.Thing.Create().SetName("placed-1").SetAt(at).Stored(&stored[0]),
			client.Thing.Create().SetName("placed-2").SetAt(at).Stored(&stored[1]),
		).Save(ctx)
		require.NoError(t, err)
		for i, v := range stored {
			require.NotNil(t, v, "thing %d", i)
			require.Equal(t, made[i].Id, v.Id)
			require.Equal(t, made[i].Name, v.Name)
			require.Equal(t, "cave", v.Motto)
		}
	})

	t.Run("and hands nothing to a row that did not ask", func(t *testing.T) {
		var stored *ent.Item
		_, err := client.Item.CreateBulk(
			client.Item.Create().SetName("unasked-1"),
			client.Item.Create().SetName("asked-2").Stored(&stored),
		).Save(ctx)
		require.NoError(t, err)
		require.NotNil(t, stored)
		require.Equal(t, "asked-2", stored.Name)
	})

	t.Run("and none of it when the batch fails", func(t *testing.T) {
		first := &ent.Item{Name: "as it was"}
		v := first
		// A name the table has already, which the column refuses: the
		// INSERT fails, and no row of it was written.
		_, err := client.Item.CreateBulk(
			client.Item.Create().SetName("fresh").Stored(&v),
			client.Item.Create().SetName("first"),
		).Save(ctx)
		require.Error(t, err)
		require.Same(t, first, v)
		require.Zero(t, client.Item.Query().Where(item.Name("fresh")).CountX(ctx))
	})
}
