// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package notnull

import (
	"context"
	"testing"

	"github.com/protobuf-orm/ent/entc/integration/notnull/ent"
	"github.com/protobuf-orm/ent/entc/integration/notnull/ent/blob"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

// TestEmptyIsNotNil is about the one value the drivers disagree about.
//
// A zero-length blob reads back as nil through one SQLite driver and as
// empty bytes through another, and the same split runs between SQLite and
// PostgreSQL. For a required column that difference is not a difference:
// the column is NOT NULL, so nil is not a value it can hold, and reading
// one out means the driver erased something rather than reported it.
//
// The cost of letting it through is a round-trip that depends on which
// database the test ran against, which is the shape of bug that reaches
// production and not the test suite.
func TestEmptyIsNotNil(t *testing.T) {
	client, err := ent.Open("sqlite3", "file:notnull?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx))

	empty := client.Blob.Create().SetData([]byte{}).SaveX(ctx)
	got := client.Blob.GetX(ctx, empty.Id)
	require.NotNil(t, got.Data, "a required column cannot hold a nil")
	require.Empty(t, got.Data)

	// An optional column can, and says so: nothing was written to it.
	require.Nil(t, got.Note)

	// Nothing about the required case is special-cased away when there is
	// something to read.
	full := client.Blob.Create().SetData([]byte("x")).SetNote([]byte("y")).SaveX(ctx)
	got = client.Blob.GetX(ctx, full.Id)
	require.Equal(t, []byte("x"), got.Data)
	require.Equal(t, []byte("y"), got.Note)

	// On an optional column the two cannot be told apart, and this is what
	// that looks like: an empty write reads back as nothing written. The
	// driver has erased a difference the column is allowed to hold, and
	// nothing here can recover it -- which is the reason a field whose
	// emptiness carries meaning should not be Optional.
	blank := client.Blob.Create().SetData([]byte("x")).SetNote([]byte{}).SaveX(ctx)
	require.Nil(t, client.Blob.GetX(ctx, blank.Id).Note)

	// And the same through a query rather than a get by id.
	rows := client.Blob.Query().Where(blob.IdEQ(empty.Id)).AllX(ctx)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Data)
}
