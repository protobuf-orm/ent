// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package noclientschema is the only package here generated with
// `sql/no-client-schema` on, and it exists to be compiled.
//
// The flag drops Client.Schema and the generated package's import of its own
// migrate, so that a binary which never migrates does not link Atlas -- its
// diff planner, three SQL dialects and an HCL parser -- to answer a question it
// does not ask. Rendering that is checked by TestGraph_Gen, which runs codegen
// with every feature on; what it cannot check is that the result builds, since
// it only asks whether the files exist.
//
// This package answers that by being an ordinary package: `go build` compiles
// it, `go generate` regenerates it with the flag on, and the generate job fails
// if either drifts.
package noclientschema

import (
	"context"
	"reflect"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/entc/integration/noclientschema/ent"
	"github.com/protobuf-orm/ent/entc/integration/noclientschema/ent/migrate"
	"github.com/protobuf-orm/ent/entc/integration/noclientschema/ent/user"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestClientWithoutSchema(t *testing.T) {
	ctx := context.Background()
	drv, err := sql.Open(dialect.SQLite, "file:"+t.TempDir()+"/db.sqlite?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer drv.Close()

	// The migration is the caller's to run, and it is run on the driver the
	// client is built from rather than through the client. That is the whole
	// of what the flag changes for a caller, and it is why `migrate` is still
	// generated: what goes is the edge from Client to it.
	require.NoError(t, migrate.NewSchema(drv).Create(ctx))

	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	u := client.User.Create().SetName("a8m").SaveX(ctx)
	require.Equal(t, u.Id, client.User.Query().Where(user.Name("a8m")).OnlyIdX(ctx))
}

// TestSchemaIsNotOnClient states the flag's effect as an assertion rather than
// as a property of the build, so that a Client which quietly grew the field
// back is caught here and not in the size of somebody's binary.
//
// By reflection, because a field that is not there cannot be named: the test
// would stop compiling instead of failing, and a test that does not compile is
// a test nobody reads the result of.
func TestSchemaIsNotOnClient(t *testing.T) {
	rt := reflect.TypeFor[ent.Client]()
	_, ok := rt.FieldByName("Schema")
	require.False(t, ok, "Client.Schema is back, and the generated package imports migrate again")
}
