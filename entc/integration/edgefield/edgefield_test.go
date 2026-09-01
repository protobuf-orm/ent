// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package edgefield

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/entc/integration/edgefield/ent"
	"github.com/protobuf-orm/ent/entc/integration/edgefield/ent/migrate"
	"github.com/protobuf-orm/ent/entc/integration/edgefield/ent/node"
	"github.com/protobuf-orm/ent/entc/integration/edgefield/ent/pet"
	"github.com/protobuf-orm/ent/entc/integration/edgefield/ent/rental"
	"github.com/protobuf-orm/ent/entc/integration/edgefield/ent/user"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestEdgeField(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	a8m := client.User.Create().SaveX(ctx)
	p1 := client.Pet.Create().SetOwner(a8m).SaveX(ctx)
	require.Equal(t, a8m.Id, p1.OwnerId)
	f1 := client.Pet.Query().Where(pet.OwnerId(a8m.Id)).OnlyX(ctx)
	require.Equal(t, p1.Id, f1.Id)
	require.Equal(t, p1.OwnerId, f1.OwnerId)

	c1 := client.User.Create().SetParent(a8m).SaveX(ctx)
	require.Equal(t, c1.ParentId, a8m.Id)
	c2 := client.User.Create().SetParentId(a8m.Id).SaveX(ctx)
	require.Equal(t, c2.ParentId, a8m.Id)
	pid := a8m.QueryChildren().GroupBy(user.FieldParentId).IntX(ctx)
	require.Equal(t, pid, a8m.Id)
	c3 := client.User.Create().SetParentId(c2.Id).SaveX(ctx)
	require.Equal(t,
		client.User.Query().
			Where(
				user.HasParentWith(
					user.ParentId(a8m.Id),
				),
			).OnlyIdX(ctx),
		c3.Id,
	)

	ps1 := client.Post.Create().SetText("entgo.io").SaveX(ctx)
	require.Nil(t, ps1.AuthorId)
	ps1 = ps1.Update().SetAuthorId(a8m.Id).SaveX(ctx)
	require.NotNil(t, ps1.AuthorId)
	require.Equal(t, a8m.Id, *ps1.AuthorId)
	ps1 = client.Post.Query().WithAuthor().OnlyX(ctx)
	require.NotNil(t, ps1.AuthorId)
	require.Equal(t, a8m.Id, *ps1.AuthorId)
	require.Equal(t, a8m.Id, ps1.Edges.Author.Id)

	nati := client.User.Create().SetSpouse(a8m).SaveX(ctx)
	require.Equal(t, nati.SpouseId, a8m.Id)
	require.Equal(t, nati.Id, a8m.QuerySpouse().OnlyIdX(ctx))

	visa := client.Card.Create().SetOwnerId(a8m.Id).SaveX(ctx)
	require.Equal(t, a8m.Id, visa.OwnerId)
	require.Equal(t, nati.Id, visa.QueryOwner().QuerySpouse().OnlyIdX(ctx))
	require.Equal(t, nati.Id, client.Card.Query().QueryOwner().QuerySpouse().OnlyIdX(ctx))

	m1 := client.Metadata.Create().SetUser(a8m).SetAge(10).SaveX(ctx)
	require.Equal(t, a8m.Id, m1.Id)
	require.Equal(t, 10, m1.Age)
	m1 = a8m.QueryMetadata().OnlyX(ctx)
	require.Equal(t, a8m.Id, m1.Id)
	require.Equal(t, a8m.Id, m1.QueryUser().OnlyIdX(ctx))
	_, err = client.Metadata.Create().SetId(a8m.Id).SetAge(10).Save(ctx)
	require.True(t, ent.IsConstraintError(err), "UNIQUE constraint failed: metadata.id")
	err = m1.Update().ClearUser().Exec(ctx)
	require.Error(t, err, "clearing primary key is not allowed")

	client.Info.Create().SetUser(a8m).SetContent(json.RawMessage("{}")).SaveX(ctx)
	inf := a8m.QueryInfo().OnlyX(ctx)
	require.Equal(t, a8m.Id, inf.Id)
	_, err = client.Info.Create().SetId(a8m.Id).SetContent(json.RawMessage("10")).Save(ctx)
	require.True(t, ent.IsConstraintError(err), "UNIQUE constraint failed: metadata.id")

	require.NotZero(t, client.Pet.Query().QueryOwner().CountX(ctx))
	client.Pet.Update().ClearOwnerId().ExecX(ctx)
	require.Zero(t, client.Pet.Query().QueryOwner().CountX(ctx))

	require.False(t, client.Rental.Query().ExistX(ctx))
	car1 := client.Car.Create().SetNumber("102030").SaveX(ctx)
	car2 := client.Car.Create().SetNumber("102030").SaveX(ctx)
	client.Rental.Create().SetUserId(a8m.Id).SetCarId(car1.Id).SaveX(ctx)
	require.Equal(t, car1.Id, a8m.QueryRentals().QueryCar().OnlyIdX(ctx))
	dt, err := time.Parse(time.RFC3339, "1906-01-02T00:00:00+00:00")
	require.NoError(t, err)
	client.Rental.Create().SetUserId(a8m.Id).SetCarId(car2.Id).SetDate(dt).SaveX(ctx)
	require.Equal(t, 2, a8m.QueryRentals().QueryCar().CountX(ctx))
	require.Equal(t, car2.Id, a8m.QueryRentals().Where(rental.DateLTE(dt)).QueryCar().OnlyIdX(ctx))
	_, err = client.Rental.Create().SetUserId(a8m.Id).SetCarId(car2.Id).SetDate(dt).Save(ctx)
	require.Error(t, err)
	require.True(t, ent.IsConstraintError(err))

	curr := client.Node.Create().SaveX(ctx)
	for i := 0; i < 5; i++ {
		curr = client.Node.Create().SetPrevId(curr.Id).SetValue(curr.Value + 1).SaveX(ctx)
	}
	head := client.Node.Query().Where(node.Not(node.HasPrev())).OnlyX(ctx)
	for i := 0; i < 5; i++ {
		curr = head.QueryNext().OnlyX(ctx)
		require.Equal(t, head.Value+1, curr.Value)
		head = curr
	}
}

func TestNamedEdges(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))
	u1 := client.User.Create().SaveX(ctx)
	client.Pet.Create().SetOwner(u1).SaveX(ctx)

	u1 = client.User.Query().
		WithPets(func(q *ent.PetQuery) {
			q.Select(pet.FieldId)
		}).
		WithNamedPets("Named", func(q *ent.PetQuery) {
			q.Select(pet.FieldId)
		}).
		OnlyX(ctx)
	require.Len(t, u1.Edges.Pets, 1)
	require.Equal(t, u1.Edges.Pets[0].OwnerId, u1.Id)
	pets, err := u1.NamedPets("Named")
	require.NoError(t, err)
	require.Len(t, pets, 1)
	require.Equal(t, pets[0].OwnerId, u1.Id)
}
