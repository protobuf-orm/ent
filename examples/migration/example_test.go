// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package migration shows how a schema change becomes a file that is reviewed,
// committed and applied, rather than something a server does to a database on
// the way up.
//
// The three calls are Plan, Apply and Current, and all of them are
// dialect/sql/schema. No CLI is involved: planning and applying use the atlas
// packages ent already depends on.
package migration

import (
	"context"
	dbsql "database/sql"
	"os"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	entschema "github.com/protobuf-orm/ent/dialect/sql/schema"
	"github.com/protobuf-orm/ent/examples/migration/ent"
	"github.com/protobuf-orm/ent/examples/migration/ent/migrate"
	"github.com/protobuf-orm/ent/examples/migration/ent/user"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestMigration(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir() + "/migrations"
	require.NoError(t, os.Mkdir(dir, 0o755))

	// Where the files live, which database they are written for, and the shape
	// they bring one to. Tables arrives as a value because it is the one thing
	// here the generated code owns.
	d, err := entschema.OpenDir(dir)
	require.NoError(t, err)
	m := entschema.Migrations{Dir: d, Dialect: dialect.SQLite, Tables: migrate.Tables}

	// Planning needs a dev database: an empty one of the same kind, onto which
	// the files already written are replayed to work out the state they reach.
	// It is written to and emptied again, so it must not be one anyone cares
	// about.
	dev := open(t, ":memory:")
	files, err := m.Plan(ctx, dev, "initial")
	require.NoError(t, err)
	require.Len(t, files, 1, "one change, one file")
	t.Logf("planned %s", files[0].Name())

	// Nothing was applied yet, so the whole directory is pending and a database
	// that has run none of it is not current.
	db := open(t, t.TempDir()+"/db.sqlite")
	pending, err := m.Pending(ctx, db, dialect.SQLite)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Error(t, m.Current(ctx, db, dialect.SQLite))

	// Applying runs every file this database has not run, in order, and records
	// each only once all of its statements have.
	applied, err := m.Apply(ctx, db, dialect.SQLite)
	require.NoError(t, err)
	require.Len(t, applied, 1)

	// Which is a question a deployment can ask on the way up.
	require.NoError(t, m.Current(ctx, db, dialect.SQLite))

	// Applying again is what a deployment that restarts does, and it is not an
	// error to have nothing to do.
	applied, err = m.Apply(ctx, db, dialect.SQLite)
	require.NoError(t, err)
	require.Empty(t, applied)

	// The database the files built is the one the client expects.
	client := ent.NewClient(ent.Driver(sql.OpenDB(dialect.SQLite, db)))
	u := client.User.Create().SetFirstName("Ariel").SetLastName("Mashraki").SetAge(30).SaveX(ctx)
	require.Equal(t, u.Id, client.User.Query().Where(user.FirstName("Ariel")).OnlyIdX(ctx))
}

func open(t *testing.T, name string) *dbsql.DB {
	t.Helper()
	db, err := dbsql.Open("sqlite3", "file:"+name+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}
