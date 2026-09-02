package stduuid

import (
	"context"
	dbsql "database/sql"
	"testing"

	"github.com/protobuf-orm/ent/entc/integration/stduuid/ent"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

// TestStoredType pins the form a UUID column holds, which is the reason the
// standard library's type is left to database/sql rather than given a codec.
//
// A codec that wrote MarshalText would put a BLOB here on SQLite, where a
// driver.Valuer returning a string put TEXT, and the two do not compare equal
// -- so every row an earlier deployment wrote would stop matching. What
// database/sql does is the same thing github.com/google/uuid's Valuer did.
func TestStoredType(t *testing.T) {
	dsn := "file:" + t.TempDir() + "/db.sqlite?_pragma=foreign_keys(1)"
	client, err := ent.Open("sqlite3", dsn)
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx))

	u := client.User.Create().SetName("x").SaveX(ctx)

	db, err := dbsql.Open("sqlite3", dsn)
	require.NoError(t, err)
	defer db.Close()
	var typ, raw string
	require.NoError(t, db.QueryRow("SELECT typeof(id), id FROM user").Scan(&typ, &raw))
	require.Equal(t, "text", typ)
	require.Equal(t, u.Id.String(), raw)
}
