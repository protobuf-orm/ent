// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package expr holds what each engine says about a field the mutation assigns
// a SQL expression: that the UPDATE writes it, that a hook sees it as it sees
// a field set to a value, and that a field whose guarantee an expression
// would pass by refuses one rather than skipping the guarantee.
package expr

import (
	"context"
	"fmt"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/entc/integration/expr/ent"
	"github.com/protobuf-orm/ent/entc/integration/expr/ent/hook"
	"github.com/protobuf-orm/ent/entc/integration/expr/ent/item"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestSQLite(t *testing.T) {
	drv, err := sql.Open(dialect.SQLite, "file:"+t.TempDir()+"/db.sqlite?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer drv.Close()
	writes(t, drv)
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
	require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS expr", []any{}, nil))
	require.NoError(t, root.Exec(ctx, "CREATE DATABASE expr", []any{}, nil))
	defer root.Exec(ctx, "DROP DATABASE IF EXISTS expr", []any{}, nil)

	drv, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/expr?parseTime=True", port))
	require.NoError(t, err)
	defer drv.Close()
	writes(t, drv)
}

func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{"14": 5434, "17": 5437} {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			dsn := fmt.Sprintf("host=localhost port=%d user=postgres password=pass sslmode=disable", port)
			root, err := sql.Open(dialect.Postgres, dsn)
			require.NoError(t, err)
			defer root.Close()
			require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS expr", []any{}, nil))
			require.NoError(t, root.Exec(ctx, "CREATE DATABASE expr", []any{}, nil))
			defer root.Exec(ctx, "DROP DATABASE IF EXISTS expr", []any{}, nil)

			drv, err := sql.Open(dialect.Postgres, dsn+" dbname=expr")
			require.NoError(t, err)
			defer drv.Close()
			writes(t, drv)
		})
	}
}

// upper is the item's name in capitals, which only the database computes:
// the mutation never holds the value it stands for.
func upper() sql.Querier {
	return sql.ExprFunc(func(b *sql.Builder) {
		b.S("UPPER(").Ident(item.FieldName).S(")")
	})
}

// writes is the same things on every engine.
func writes(t *testing.T, drv *sql.Driver) {
	ctx := context.Background()
	client := ent.NewClient(ent.Driver(drv))
	require.NoError(t, client.Schema.Create(ctx))

	create := func(name string) *ent.Item {
		return client.Item.Create().SetName(name).SetCode(name).SaveX(ctx)
	}

	t.Run("the UPDATE writes it, and the row read back is what it made", func(t *testing.T) {
		it := create("written")
		u := client.Item.UpdateOne(it)
		require.NoError(t, u.Mutation().SetFieldExpr(item.FieldLabel, upper()))
		got := u.SaveX(ctx)
		require.Equal(t, "WRITTEN", got.Label)
		require.Equal(t, "WRITTEN", client.Item.GetX(ctx, it.Id).Label)
	})

	t.Run("so does an update of many", func(t *testing.T) {
		it := create("many")
		u := client.Item.Update().Where(item.Id(it.Id))
		require.NoError(t, u.Mutation().SetFieldExpr(item.FieldLabel, upper()))
		require.Equal(t, 1, u.SaveX(ctx))
		require.Equal(t, "MANY", client.Item.GetX(ctx, it.Id).Label)
	})

	t.Run("a hook sees the field, and its value is the expression", func(t *testing.T) {
		it := create("hooked")
		var (
			fields []string
			value  any
		)
		// A client of its own, so that the hook sees this update and no other.
		hooked := ent.NewClient(ent.Driver(drv))
		hooked.Item.Use(func(next ent.Mutator) ent.Mutator {
			return hook.ItemFunc(func(ctx context.Context, m *ent.ItemMutation) (ent.Value, error) {
				fields = m.Fields()
				value, _ = m.Field(item.FieldNote)
				return next.Mutate(ctx, m)
			})
		})
		u := hooked.Item.UpdateOne(it)
		require.NoError(t, u.Mutation().SetFieldExpr(item.FieldNote, upper()))
		require.Equal(t, "HOOKED", *u.SaveX(ctx).Note)
		require.Contains(t, fields, item.FieldNote)
		require.Implements(t, (*sql.Querier)(nil), value)
	})

	t.Run("what Field returns, SetField takes back", func(t *testing.T) {
		it := create("copied")
		u := client.Item.UpdateOne(it)
		m := u.Mutation()
		require.NoError(t, m.SetFieldExpr(item.FieldLabel, upper()))
		v, ok := m.Field(item.FieldLabel)
		require.True(t, ok)
		require.NoError(t, m.SetField(item.FieldNote, v))
		got := u.SaveX(ctx)
		require.Equal(t, "COPIED", *got.Note)
	})

	// Two assignments to one column are an error in PostgreSQL and the later
	// one silently wins elsewhere, so whichever came last is the only one.
	t.Run("it and a clear replace each other", func(t *testing.T) {
		it := create("cleared")
		client.Item.UpdateOne(it).SetNote("was").ExecX(ctx)

		u := client.Item.UpdateOne(it).ClearNote()
		require.NoError(t, u.Mutation().SetFieldExpr(item.FieldNote, upper()))
		require.False(t, u.Mutation().NoteCleared())
		require.Equal(t, "CLEARED", *u.SaveX(ctx).Note)

		u = client.Item.UpdateOne(it)
		require.NoError(t, u.Mutation().SetFieldExpr(item.FieldNote, upper()))
		u.ClearNote()
		_, ok := u.Mutation().FieldExpr(item.FieldNote)
		require.False(t, ok)
		require.Nil(t, u.SaveX(ctx).Note)
	})

	t.Run("it and a value replace each other", func(t *testing.T) {
		it := create("replaced")

		u := client.Item.UpdateOne(it).AddRevision(5)
		require.NoError(t, u.Mutation().SetFieldExpr(item.FieldRevision, sql.ExprFunc(func(b *sql.Builder) {
			b.Ident(item.FieldRevision).S(" + ").Arg(10)
		})))
		_, added := u.Mutation().AddedRevision()
		require.False(t, added)
		require.Equal(t, 10, u.SaveX(ctx).Revision)

		u = client.Item.UpdateOne(it)
		require.NoError(t, u.Mutation().SetFieldExpr(item.FieldLabel, upper()))
		got := u.SetLabel("value").SaveX(ctx)
		require.Equal(t, "value", got.Label)
	})

	t.Run("an update default does not replace it", func(t *testing.T) {
		it := create("defaulted")
		require.Equal(t, -1, client.Item.UpdateOne(it).SetLabel("x").SaveX(ctx).Revision)

		u := client.Item.UpdateOne(it)
		require.NoError(t, u.Mutation().SetFieldExpr(item.FieldRevision, sql.ExprFunc(func(b *sql.Builder) {
			b.Arg(7)
		})))
		require.Equal(t, 7, u.SaveX(ctx).Revision)
	})

	// A JSON column's value is encoded on its way to the database. An
	// expression is not a value, and encoding it would write what
	// encoding/json makes of the Go struct behind it.
	t.Run("a JSON column writes it rather than encoding it", func(t *testing.T) {
		it := create("json")
		u := client.Item.UpdateOne(it)
		require.NoError(t, u.Mutation().SetFieldExpr(item.FieldTags, sql.ExprFunc(func(b *sql.Builder) {
			b.Arg(`["a","b"]`)
		})))
		require.Equal(t, []string{"a", "b"}, u.SaveX(ctx).Tags)
	})

	t.Run("a field it would pass by refuses it", func(t *testing.T) {
		it := create("refused")
		m := client.Item.UpdateOne(it).Mutation()

		// MaxLen is a validator, and an expression has nothing to hand it.
		require.ErrorContains(t, m.SetFieldExpr(item.FieldName, upper()), "validated")
		require.ErrorContains(t, m.SetField(item.FieldName, upper()), "validated")
		// An update does not write an immutable field.
		require.ErrorContains(t, m.SetFieldExpr(item.FieldCode, upper()), "immutable")
		require.ErrorContains(t, m.SetFieldExpr("missing", upper()), "unknown")
		require.Error(t, m.SetFieldExpr(item.FieldLabel, nil))
		require.Empty(t, m.Fields())
	})

	t.Run("a create refuses it", func(t *testing.T) {
		c := client.Item.Create().SetName("created").SetCode("created")
		require.ErrorContains(t, c.Mutation().SetFieldExpr(item.FieldLabel, upper()), "create")

		// A mutation that became a create after it was given one is refused
		// when it is saved, rather than written without it.
		c = client.Item.Create().SetName("sneaked").SetCode("sneaked")
		c.Mutation().SetOp(ent.OpUpdateOne)
		require.NoError(t, c.Mutation().SetFieldExpr(item.FieldLabel, upper()))
		c.Mutation().SetOp(ent.OpCreate)
		_, err := c.Save(ctx)
		require.True(t, ent.IsValidationError(err), "%v", err)
		require.Zero(t, client.Item.Query().Where(item.Name("sneaked")).CountX(ctx))
	})
}
