// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package integration

import (
	"context"
	"database/sql/driver"
	"testing"

	"uuid"

	"github.com/protobuf-orm/ent/entc/integration/ent"
	"github.com/protobuf-orm/ent/entc/integration/ent/pet"
	"github.com/protobuf-orm/ent/entc/integration/ent/user"
	"github.com/protobuf-orm/ent/entql"

	"github.com/stretchr/testify/require"
)

// storedUuid is a uuid.UUID as a driver.Valuer.
//
// database/sql converts one on its own, but entql asks for a Valuer at the
// call rather than at the bind, and the standard library's type is not one.
type storedUuid uuid.UUID

func (u storedUuid) Value() (driver.Value, error) {
	return driver.DefaultParameterConverter.ConvertValue(uuid.UUID(u))
}

func EntQL(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	a8m := client.User.Create().SetName("a8m").SetAge(30).SaveX(ctx)
	nati := client.User.Create().SetName("nati").SetAge(30).AddFriends(a8m).SaveX(ctx)

	uq := client.User.Query()
	uq.Filter().Where(entql.HasEdge("friends"))
	require.Equal(2, uq.CountX(ctx))

	uq = client.User.Query()
	uq.Filter().Where(
		entql.And(
			entql.FieldEQ("name", "nati"),
			entql.HasEdge("friends"),
		),
	)
	require.Equal(nati.Id, uq.OnlyIdX(ctx))

	u1, u2 := uuid.New(), uuid.New()
	xabi := client.Pet.Create().SetName("xabi").SetOwner(a8m).SetUuid(u1).SaveX(ctx)
	luna := client.Pet.Create().SetName("luna").SetOwner(nati).SetUuid(u2).SaveX(ctx)
	uq = client.User.Query()
	uq.Filter().Where(
		entql.And(
			entql.HasEdge("pets"),
			entql.HasEdgeWith("friends", entql.FieldEQ("name", "nati")),
			entql.HasEdgeWith("friends", entql.FieldIn("name", "nati")),
			entql.HasEdgeWith("friends", entql.FieldIn("name", "nati", "a8m")),
		),
	)
	require.Equal(a8m.Id, uq.OnlyIdX(ctx))
	uq = client.User.Query()
	uq.Filter().Where(
		entql.And(
			entql.HasEdgeWith("pets", entql.FieldEQ("name", "luna")),
			entql.HasEdge("friends"),
		),
	)
	require.Equal(nati.Id, uq.OnlyIdX(ctx))

	pq := client.Pet.Query()
	pq.Filter().WhereUuid(entql.ValueEQ(storedUuid(u1)))
	require.Equal(xabi.Id, pq.OnlyIdX(ctx))
	pq = client.Pet.Query()
	pq.Filter().WhereUuid(entql.ValueEQ(storedUuid(u2)))
	require.Equal(luna.Id, pq.OnlyIdX(ctx))

	uq = client.User.Query()
	uq.Filter().WhereName(entql.StringEQ("a8m"))
	require.Equal(a8m.Id, uq.OnlyIdX(ctx))
	pq = client.Pet.Query()
	pq.Filter().WhereName(entql.StringOr(entql.StringEQ("xabi"), entql.StringEQ("luna")))
	require.Equal([]int{luna.Id, xabi.Id}, pq.Order(ent.Asc(pet.FieldName)).IdsX(ctx))

	pq = client.Pet.Query()
	pq.Where(pet.Name(luna.Name)).Filter().WhereId(entql.IntEQ(luna.Id))
	require.Equal(luna.Id, pq.Order(ent.Asc(pet.FieldName)).OnlyIdX(ctx))
	pq = client.Pet.Query()
	pq.Where(pet.Name(luna.Name)).Filter().WhereId(entql.IntEQ(xabi.Id))
	require.False(pq.ExistX(ctx))

	update := client.User.Update().SetRole(user.RoleAdmin)
	update.Mutation().Filter().WhereName(entql.StringEQ(a8m.Name))
	updated := update.SaveX(ctx)
	require.Equal(1, updated)
	uq = client.User.Query()
	uq.Filter().WhereRole(entql.StringEQ(string(user.RoleAdmin)))
	require.Equal(a8m.Id, uq.OnlyIdX(ctx))

	uq = client.User.Query()
	uq.Filter().WhereName(entql.StringEQ(a8m.Name))
	uq = uq.QueryFriends()
	uq.Filter().WhereName(entql.StringEQ(nati.Name))
	require.Equal(luna.Id, uq.QueryPets().OnlyIdX(ctx))
}
