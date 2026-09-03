// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package stduuid

import (
	"context"
	"testing"

	entgo "github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/entc/integration/stduuid/ent"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func open(t *testing.T) (*ent.Client, context.Context) {
	t.Helper()
	// A file rather than a shared-cache memory database: these tests hold a
	// transaction open while querying beside it, and two connections into an
	// in-memory database are two databases as soon as the first one closes.
	client, err := ent.Open("sqlite3", "file:"+t.TempDir()+"/db.sqlite?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx))
	return client, ctx
}

// TestInTx is one question with one answer, however the transaction was begun.
//
// There were two of each. A client knew about the transaction its own Tx hands
// out and nothing else, so a stack sharing a transaction begun at the driver --
// which is how one call spanning several servers becomes one write -- was told
// it was not in a transaction, and started a second one inside it for nothing.
// A driver-level check written outside knew the opposite half, and neither
// looked through the wrappers a driver picks up.
func TestInTx(t *testing.T) {
	t.Run("a fresh client is in none", func(t *testing.T) {
		client, _ := open(t)
		require.False(t, client.InTx())
		require.False(t, dialect.InTx(client.Driver()))
	})

	t.Run("one begun at the client", func(t *testing.T) {
		client, ctx := open(t)
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		defer tx.Rollback()

		require.True(t, tx.Client().InTx())
		require.True(t, dialect.InTx(tx.Client().Driver()))
	})

	t.Run("one begun at the driver, which a stack shares", func(t *testing.T) {
		client, ctx := open(t)
		drv, tx, err := dialect.BeginTx(ctx, client.Driver())
		require.NoError(t, err)
		defer tx.Rollback()

		in := client.WithDriver(drv)
		require.True(t, in.InTx())
		require.True(t, dialect.InTx(in.Driver()))
	})

	t.Run("and through a wrapper", func(t *testing.T) {
		client, ctx := open(t)
		drv, tx, err := dialect.BeginTx(ctx, client.Driver())
		require.NoError(t, err)
		defer tx.Rollback()

		in := client.WithDriver(dialect.Debug(drv, func(...any) {}))
		require.True(t, in.InTx())
		require.True(t, dialect.InTx(in.Driver()))
	})
}

// TestBeginTx is the transaction a stack shares: several clients on one driver,
// one commit, and no inner call able to end it early.
func TestBeginTx(t *testing.T) {
	client, ctx := open(t)

	drv, tx, err := dialect.BeginTx(ctx, client.Driver())
	require.NoError(t, err)

	one := client.WithDriver(drv)
	two := client.WithDriver(drv)

	a := one.User.Create().SetName("a").SaveX(ctx)
	two.User.Create().SetName("b").SaveX(ctx)

	// An inner call opening a transaction of its own joins this one. Ending
	// it must not end this one, or the work above is thrown away and the call
	// that wrapped it is told it succeeded.
	inner, err := one.Tx(ctx)
	require.NoError(t, err)
	inner.User.Create().SetName("c").SaveX(ctx)
	require.NoError(t, inner.Rollback())

	require.Equal(t, 3, one.User.Query().CountX(ctx), "the rollback took nothing")
	require.NoError(t, tx.Commit())

	require.Equal(t, 3, client.User.Query().CountX(ctx))
	require.Equal(t, "a", client.User.GetX(ctx, a.Id).Name)
}

// TestJoinTx is the shape a caller writes whether or not there is anything to
// join, which is the point of it.
func TestJoinTx(t *testing.T) {
	t.Run("starts one when there is none and it is wanted", func(t *testing.T) {
		client, ctx := open(t)

		j, err := entgo.JoinTx[*ent.Client, *ent.Tx](ctx, client, true)
		require.NoError(t, err)
		require.True(t, j.Db.InTx())

		j.Db.User.Create().SetName("a").SaveX(ctx)
		require.Equal(t, 0, client.User.Query().CountX(ctx), "not yet")
		require.NoError(t, j.Commit())
		require.Equal(t, 1, client.User.Query().CountX(ctx))

		// Deferred after a commit, and it takes nothing back.
		j.Close()
		require.Equal(t, 1, client.User.Query().CountX(ctx))
	})

	t.Run("takes the work back when nothing committed", func(t *testing.T) {
		client, ctx := open(t)

		j, err := entgo.JoinTx[*ent.Client, *ent.Tx](ctx, client, true)
		require.NoError(t, err)
		j.Db.User.Create().SetName("a").SaveX(ctx)
		j.Close()

		require.Equal(t, 0, client.User.Query().CountX(ctx))
	})

	t.Run("starts none when it is not wanted", func(t *testing.T) {
		client, ctx := open(t)

		j, err := entgo.JoinTx[*ent.Client, *ent.Tx](ctx, client, false)
		require.NoError(t, err)
		require.False(t, j.Db.InTx())

		j.Db.User.Create().SetName("a").SaveX(ctx)
		require.NoError(t, j.Commit())

		// Close after a commit that did nothing takes nothing back either:
		// there was no transaction, so the write was never provisional.
		j.Close()
		require.Equal(t, 1, client.User.Query().CountX(ctx))
	})

	t.Run("joins one somebody else began, and does not end it", func(t *testing.T) {
		client, ctx := open(t)

		drv, tx, err := dialect.BeginTx(ctx, client.Driver())
		require.NoError(t, err)
		in := client.WithDriver(drv)

		j, err := entgo.JoinTx[*ent.Client, *ent.Tx](ctx, in, true)
		require.NoError(t, err)
		j.Db.User.Create().SetName("a").SaveX(ctx)

		// Neither ends what it joined. A Close that rolled back here would
		// throw away the work of the call that began the transaction.
		require.NoError(t, j.Commit())
		j.Close()

		require.NoError(t, tx.Commit())
		require.Equal(t, 1, client.User.Query().CountX(ctx))
	})
}
