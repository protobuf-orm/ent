// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package fieldnames holds a schema whose fields are named after the methods
// every mutation has, and what each engine says of the client it generates:
// that it compiles, which it did not, and that the mutation's methods that
// take a field by its name -- the ones a hook or a privacy rule calls -- reach
// that field's own, whatever they had to be renamed to.
package fieldnames

import (
	"context"
	"fmt"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/entc/integration/fieldnames/ent"
	"github.com/protobuf-orm/ent/entc/integration/fieldnames/ent/item"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestSQLite(t *testing.T) {
	drv, err := sql.Open(dialect.SQLite, "file:"+t.TempDir()+"/db.sqlite?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer drv.Close()
	named(t, drv)
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
	require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS fieldnames", []any{}, nil))
	require.NoError(t, root.Exec(ctx, "CREATE DATABASE fieldnames", []any{}, nil))
	defer root.Exec(ctx, "DROP DATABASE IF EXISTS fieldnames", []any{}, nil)

	drv, err := sql.Open(dialect.MySql, fmt.Sprintf("root:pass@tcp(localhost:%d)/fieldnames", port))
	require.NoError(t, err)
	defer drv.Close()
	named(t, drv)
}

func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{"14": 5434, "17": 5437} {
		t.Run(version, func(t *testing.T) {
			ctx := context.Background()
			dsn := fmt.Sprintf("host=localhost port=%d user=postgres password=pass sslmode=disable", port)
			root, err := sql.Open(dialect.Postgres, dsn)
			require.NoError(t, err)
			defer root.Close()
			require.NoError(t, root.Exec(ctx, "DROP DATABASE IF EXISTS fieldnames", []any{}, nil))
			require.NoError(t, root.Exec(ctx, "CREATE DATABASE fieldnames", []any{}, nil))
			defer root.Exec(ctx, "DROP DATABASE IF EXISTS fieldnames", []any{}, nil)

			drv, err := sql.Open(dialect.Postgres, dsn+" dbname=fieldnames")
			require.NoError(t, err)
			defer drv.Close()
			named(t, drv)
		})
	}
}

// names are the Item's fields, in the order [values] reads them.
var names = []string{
	item.FieldField, item.FieldFields, item.FieldEdge, item.FieldIds,
	item.FieldWhere, item.FieldOp, item.FieldType,
}

func values(it *ent.Item) []*int {
	return []*int{it.Field, it.Fields, it.Edge, it.Ids, it.Where, it.Op, it.Type}
}

// holds is whether every field of the row is n, and nothing when n is nil.
func holds(t *testing.T, n *int, it *ent.Item) {
	t.Helper()
	for i, v := range values(it) {
		require.Equal(t, n, v, names[i])
	}
}

// named is the same things on every engine.
func named(t *testing.T, drv *sql.Driver) {
	ctx := context.Background()
	client := ent.NewClient(ent.Driver(drv))
	require.NoError(t, client.Schema.Create(ctx))

	// create writes n to every field of a new row.
	create := func(n int) *ent.Item {
		return client.Item.Create().
			SetField(n).SetFields(n).SetEdge(n).SetIds(n).SetWhere(n).SetOp(n).SetType(n).
			SaveX(ctx)
	}
	of := func(n int) *int { return &n }

	t.Run("a field is written, read, and filtered and ordered by", func(t *testing.T) {
		a, b := create(1), create(2)
		holds(t, of(2), b)

		got := client.Item.Query().
			Where(item.Field(2), item.Fields(2), item.Edge(2), item.Ids(2), item.Where(2), item.Op(2), item.Type(2)).
			OnlyX(ctx)
		require.Equal(t, b.Id, got.Id)
		holds(t, of(2), got)

		for i, by := range []func(...sql.OrderTermOption) item.OrderOption{
			item.ByField, item.ByFields, item.ByEdge, item.ByIds, item.ByWhere, item.ByOp, item.ByType,
		} {
			ids := client.Item.Query().Where(item.IdIn(a.Id, b.Id)).Order(by(sql.OrderDesc())).IdsX(ctx)
			require.Equal(t, []int{b.Id, a.Id}, ids, names[i])
		}
	})

	t.Run("an update adds to a field and clears it", func(t *testing.T) {
		it := create(1)
		holds(t, of(3), client.Item.UpdateOne(it).
			AddFieldField(2).AddFields(2).AddEdge(2).AddIds(2).AddWhere(2).AddOp(2).AddType(2).
			SaveX(ctx))
		holds(t, of(3), client.Item.GetX(ctx, it.Id))

		n := client.Item.Update().Where(item.Id(it.Id)).
			ClearField().ClearFields().ClearEdge().ClearIds().ClearWhere().ClearOp().ClearType().
			SaveX(ctx)
		require.Equal(t, 1, n)
		holds(t, nil, client.Item.Query().Where(item.Id(it.Id), item.FieldIsNil(), item.TypeIsNil()).OnlyX(ctx))
	})

	t.Run("a hook reaches each field by its name", func(t *testing.T) {
		// What the mutation answers, by name, after the hook did op to each:
		// what was added, if anything was, and whether it was cleared.
		type answer struct {
			added   any
			cleared bool
		}
		var (
			op      func(m ent.Mutation, name string) error
			answers map[string]answer
		)
		hooked := ent.NewClient(ent.Driver(drv))
		hooked.Item.Use(func(next ent.Mutator) ent.Mutator {
			return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
				answers = map[string]answer{}
				for _, name := range names {
					if err := op(m, name); err != nil {
						return nil, err
					}
					added, ok := m.AddedField(name)
					if !ok {
						added = nil
					}
					answers[name] = answer{added, m.FieldCleared(name)}
				}
				return next.Mutate(ctx, m)
			})
		})

		it := create(1)
		op = func(m ent.Mutation, name string) error { return m.AddField(name, 2) }
		holds(t, of(3), hooked.Item.UpdateOne(it).SaveX(ctx))
		for _, name := range names {
			require.Equal(t, answer{added: 2}, answers[name], name)
		}

		op = func(m ent.Mutation, name string) error { return m.ClearField(name) }
		holds(t, nil, hooked.Item.UpdateOne(it).SaveX(ctx))
		for _, name := range names {
			require.Equal(t, answer{cleared: true}, answers[name], name)
		}

		op = func(m ent.Mutation, name string) error { return m.SetField(name, 5) }
		holds(t, of(5), hooked.Item.UpdateOne(it).SaveX(ctx))
	})

	t.Run("an update asks whether what it fills or needs was cleared", func(t *testing.T) {
		it := create(1)
		o := client.Owner.Create().SetEdge(it).SaveX(ctx)
		require.Nil(t, o.Field)

		// The update default fills `field`, unless the update clears it.
		require.Equal(t, of(1), client.Owner.UpdateOne(o).SaveX(ctx).Field)
		require.Nil(t, client.Owner.UpdateOne(o).ClearField().SaveX(ctx).Field)

		// The edge is required: an update may move it, and refuses to clear it.
		moved := create(2)
		require.Equal(t, moved.Id, client.Owner.UpdateOne(o).SetEdge(moved).SaveX(ctx).QueryEdge().OnlyIdX(ctx))
		_, err := client.Owner.UpdateOne(o).ClearEdgeEdge().SetEdge(it).Save(ctx)
		require.ErrorContains(t, err, `clearing a required unique edge "Owner.edge"`)
	})
}
