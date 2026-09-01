// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package stduuid

import (
	"context"
	"testing"
	"uuid"

	"entgo.io/ent/entc/integration/stduuid/ent"
	"entgo.io/ent/entc/integration/stduuid/ent/user"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

// TestStdUUID exercises a schema whose id and fields are the uuid.UUID of the
// standard library, which reaches the database through a ValueScanner because
// it implements neither driver.Valuer nor sql.Scanner.
func TestStdUUID(t *testing.T) {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx))

	ref := uuid.New()
	a8m := client.User.Create().SetName("a8m").SetRef(ref).SaveX(ctx)
	require.NotEqual(t, uuid.Nil(), a8m.ID, "the default should have generated an id")
	require.Equal(t, ref, a8m.Ref)

	// Reading the row back goes through FromValue.
	got := client.User.GetX(ctx, a8m.ID)
	require.Equal(t, a8m.ID, got.ID)
	require.Equal(t, ref, got.Ref)
	require.Equal(t, "a8m", got.Name)

	// The values are stored in their text form.
	var raw []struct {
		ID  string `sql:"id"`
		Ref string `sql:"ref"`
	}
	client.User.Query().
		Where(user.ID(a8m.ID)).
		Select(user.FieldID, user.FieldRef).
		ScanX(ctx, &raw)
	require.Len(t, raw, 1)
	require.Equal(t, a8m.ID.String(), raw[0].ID)
	require.Equal(t, ref.String(), raw[0].Ref)

	// Predicates convert their arguments through the ValueScanner.
	require.True(t, client.User.Query().Where(user.Ref(ref)).ExistX(ctx))
	require.False(t, client.User.Query().Where(user.Ref(uuid.New())).ExistX(ctx))
	require.True(t, client.User.Query().Where(user.RefIn(ref, uuid.New())).ExistX(ctx))
	require.True(t, client.User.Query().Where(user.IDEQ(a8m.ID)).ExistX(ctx))
	require.False(t, client.User.Query().Where(user.IDEQ(uuid.New())).ExistX(ctx))

	// Edges keyed by the same id type.
	neta := client.User.Create().SetName("neta").SetSpouse(a8m).SaveX(ctx)
	require.Equal(t, a8m.ID, neta.QuerySpouse().OnlyX(ctx).ID)
}
