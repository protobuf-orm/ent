// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package template

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/entc/integration/template/ent"
	"github.com/protobuf-orm/ent/entc/integration/template/ent/hook"
	"github.com/protobuf-orm/ent/entc/integration/template/ent/migrate"
	"github.com/protobuf-orm/ent/entc/integration/template/ent/pet"
	"github.com/protobuf-orm/ent/entc/integration/template/ent/user"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestCustomTemplate(t *testing.T) {
	client, err := ent.Open(
		"sqlite3",
		"file:ent?mode=memory&cache=shared&_fk=1",
		// Custom config option.
		ent.HttpClient(http.DefaultClient),
	)
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))
	client.User.Use(func(next ent.Mutator) ent.Mutator {
		return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
			// Access the injected HTTP client here.
			_ = m.HttpClient
			return next.Mutate(ctx, m)
		})
	})

	p := client.Pet.Create().SetAge(1).SaveX(ctx)
	u := client.User.Create().SetName("a8m").AddPets(p).SaveX(ctx)
	g := client.Group.Create().SetMaxUsers(10).SaveX(ctx)

	node, err := client.Node(ctx, p.Id)
	require.NoError(t, err)
	require.Equal(t, p.Id, node.Id)
	require.Equal(t, &ent.Field{Type: "int", Name: "Age", Value: "1"}, node.Fields[0])
	require.Equal(t, &ent.Edge{Type: "User", Name: "Owner", Ids: []int{u.Id}}, node.Edges[0])

	node, err = client.Node(ctx, u.Id)
	require.NoError(t, err)
	require.Equal(t, u.Id, node.Id)
	require.Equal(t, &ent.Field{Type: "string", Name: "Name", Value: "\"a8m\""}, node.Fields[0])
	require.Equal(t, &ent.Edge{Type: "Pet", Name: "Pets", Ids: []int{p.Id}}, node.Edges[0])

	node, err = client.Node(ctx, g.Id)
	require.NoError(t, err)
	require.Equal(t, g.Id, node.Id)
	require.Equal(t, &ent.Field{Type: "int", Name: "MaxUsers", Value: "10"}, node.Fields[0])

	// check for client additional fields.
	require.True(t, reflect.ValueOf(client).Elem().FieldByName("tables").IsValid())

	result := client.User.Query().Where(user.NameGlob("a8*")).
		AllX(ctx)
	require.Equal(t, 1, len(result))

	var v []struct{ Id, Owner int }
	client.Pet.Query().
		Modify(func(s *sql.Selector) {
			t := sql.Table(user.Table)
			s.Join(t).On(s.C(pet.OwnerColumn), t.C(user.FieldId))
			s.Select(s.C(pet.FieldId), sql.As(t.C(user.FieldId), "owner"))
		}).
		Select().
		ScanX(ctx, &v)
	require.Equal(t, p.Id, v[0].Id)
	require.Equal(t, u.Id, v[0].Owner)

	var sum int
	for _, age := range client.Pet.Query().Select(pet.FieldAge).IntsX(ctx) {
		sum += age
	}
	got := client.Pet.Query().
		Modify(func(s *sql.Selector) {
			s.Select(sql.Sum(pet.FieldAge))
		}).
		Select().
		IntX(ctx)
	require.Equal(t, sum, got)

	require.Equal(t, 20, client.HiddenData())
}
