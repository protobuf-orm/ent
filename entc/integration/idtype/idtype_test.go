// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package idtype

import (
	"context"
	"testing"

	"github.com/protobuf-orm/ent/entc/integration/idtype/ent"
	"github.com/protobuf-orm/ent/entc/integration/idtype/ent/migrate"
	"github.com/protobuf-orm/ent/entc/integration/idtype/ent/user"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestIDType(t *testing.T) {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	a8m := client.User.Create().SetName("a8m").SaveX(ctx)
	require.Equal(t, "a8m", a8m.Name)
	neta := client.User.Create().SetName("neta").SetSpouse(a8m).SaveX(ctx)
	require.Equal(t, "neta", neta.Name)
	require.Equal(t, []string{a8m.Name}, neta.QuerySpouse().Select(user.FieldName).StringsX(ctx))
}
