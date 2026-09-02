// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package stduuid

import (
	"context"
	"testing"
	"uuid"

	"github.com/protobuf-orm/ent/entc/integration/stduuid/ent"
	"github.com/protobuf-orm/ent/entc/integration/stduuid/ent/user"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

// TestStdUuid exercises a schema whose id and fields are the uuid.UUID of the
// standard library, which database/sql reads and writes itself.
func TestStdUuid(t *testing.T) {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx))

	ref := uuid.New()
	a8m := client.User.Create().SetName("a8m").SetRef(ref).SaveX(ctx)
	require.NotEqual(t, uuid.Nil(), a8m.Id, "the default should have generated an id")
	require.Equal(t, ref, a8m.Ref)

	// Reading the row back scans straight into the field.
	got := client.User.GetX(ctx, a8m.Id)
	require.Equal(t, a8m.Id, got.Id)
	require.Equal(t, ref, got.Ref)
	require.Equal(t, "a8m", got.Name)

	// The values are stored in their text form. See TestStoredType.
	var raw []struct {
		Id  string `sql:"id"`
		Ref string `sql:"ref"`
	}
	client.User.Query().
		Where(user.Id(a8m.Id)).
		Select(user.FieldId, user.FieldRef).
		ScanX(ctx, &raw)
	require.Len(t, raw, 1)
	require.Equal(t, a8m.Id.String(), raw[0].Id)
	require.Equal(t, ref.String(), raw[0].Ref)

	// Predicates bind the value itself; database/sql does the converting.
	require.True(t, client.User.Query().Where(user.Ref(ref)).ExistX(ctx))
	require.False(t, client.User.Query().Where(user.Ref(uuid.New())).ExistX(ctx))
	require.True(t, client.User.Query().Where(user.RefIn(ref, uuid.New())).ExistX(ctx))
	require.True(t, client.User.Query().Where(user.IdEQ(a8m.Id)).ExistX(ctx))
	require.False(t, client.User.Query().Where(user.IdEQ(uuid.New())).ExistX(ctx))

	// Edges keyed by the same id type. Reading the row back reads the
	// foreign-key column too, which holds the converted form and is not a
	// field of the schema, so nothing about the field says how to scan it.
	neta := client.User.Create().SetName("neta").SetSpouse(a8m).SaveX(ctx)
	require.Equal(t, a8m.Id, neta.QuerySpouse().OnlyX(ctx).Id)
}
