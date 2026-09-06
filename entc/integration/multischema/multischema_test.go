// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package multischema

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/dialect/sql/schema"
	"github.com/protobuf-orm/ent/entc/integration/multischema/ent"
	"github.com/protobuf-orm/ent/entc/integration/multischema/ent/group"
	"github.com/protobuf-orm/ent/entc/integration/multischema/ent/migrate"
	"github.com/protobuf-orm/ent/entc/integration/multischema/ent/pet"
	"github.com/protobuf-orm/ent/entc/integration/multischema/ent/user"
	"github.com/protobuf-orm/ent/entc/integration/multischema/versioned"
	vgroup "github.com/protobuf-orm/ent/entc/integration/multischema/versioned/group"
	vmigrate "github.com/protobuf-orm/ent/entc/integration/multischema/versioned/migrate"
	vpet "github.com/protobuf-orm/ent/entc/integration/multischema/versioned/pet"
	vuser "github.com/protobuf-orm/ent/entc/integration/multischema/versioned/user"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestMySql(t *testing.T) {
	db, err := sql.Open("mysql", "root:pass@tcp(localhost:3308)/?parseTime=true&multiStatements=true")
	require.NoError(t, err)
	ctx := context.Background()
	t.Cleanup(func() {
		db.ExecContext(ctx, "SET foreign_key_checks = 0")
		db.ExecContext(ctx, "DROP DATABASE IF EXISTS db1")
		db.ExecContext(ctx, "DROP DATABASE IF EXISTS db2")
		db.ExecContext(ctx, "SET foreign_key_checks = 1")
		db.Close()
	})

	migrate.ParentTable.Schema = "db1"
	migrate.PetTable.Schema = "db1"
	migrate.UserTable.Schema = "db1"
	migrate.GroupTable.Schema = "db2"
	migrate.GroupUsersTable.Schema = "db2"
	migrate.FriendshipTable.Schema = "db2"

	pl, err := schema.Dump(ctx, dialect.MySql, "8.0.19", migrate.Tables)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, pl)
	require.NoError(t, err)

	// Default schema for the connection is db1.
	db1, err := sql.Open("mysql", "root:pass@tcp(localhost:3308)/db1?parseTime=true")
	require.NoError(t, err)
	defer db1.Close()

	cfg := ent.SchemaConfig{
		// The "users" and the "pets" table reside in the same schema
		// as the connection (the default search schema). Thus, there
		// is no need to set them explicitly both in the migration and
		// runtime statements.
		//
		// Pet:  "db1",
		// User: "db1",

		// The "groups", "group_users" and "friendship" reside in external schema (db2).
		Group:      "db2",
		GroupUsers: "db2",
		// An edge with the "Through" definition is set on its edge-schema.
		Friendship: "db2",
	}
	client := ent.NewClient(ent.Driver(db1), ent.AlternateSchema(cfg))
	pedro := client.Pet.Create().SetName("Pedro").SaveX(ctx)
	groups := client.Group.CreateBulk(
		client.Group.Create().SetName("GitHub"),
		client.Group.Create().SetName("GitLab"),
	).SaveX(ctx)
	a8m := client.User.Create().SetName("a8m").AddPets(pedro).AddGroups(groups...).SaveX(ctx)

	// Custom modifier with schema config.
	var names []struct {
		User string `sql:"user_name"`
		Pet  string `sql:"pet_name"`
	}
	client.Pet.Query().
		Modify(func(s *sql.Selector) {
			// The below function is exported using a custom
			// template defined in ent/template/config.tmpl.
			cfg := ent.SchemaConfigFromContext(s.Context())
			t := sql.Table(user.Table).Schema(cfg.User)
			s.Join(t).On(s.C(pet.FieldOwnerId), t.C(user.FieldId))
			s.Select(
				sql.As(t.C(user.FieldName), "user_name"),
				sql.As(s.C(pet.FieldName), "pet_name"),
			)
		}).
		ScanX(ctx, &names)
	require.Len(t, names, 1)
	require.Equal(t, "a8m", names[0].User)
	require.Equal(t, "Pedro", names[0].Pet)

	id := client.Group.Query().
		Where(group.HasUsersWith(user.Id(a8m.Id))).
		Limit(1).
		QueryUsers().
		QueryPets().
		OnlyIdX(ctx)
	require.Equal(t, pedro.Id, id)

	affected := client.Group.
		Update().
		ClearUsers().
		Where(
			group.And(
				group.Name(groups[0].Name),
				group.HasUsersWith(
					user.HasPetsWith(
						pet.Name(pedro.Name),
					),
				),
			),
		).
		SaveX(ctx)
	require.Equal(t, 1, affected)

	exist := groups[0].QueryUsers().ExistX(ctx)
	require.False(t, exist)
	exist = groups[1].QueryUsers().ExistX(ctx)
	require.True(t, exist)
	exist = pedro.QueryOwner().ExistX(ctx)
	require.True(t, exist)
	pedro = pedro.Update().ClearOwner().SaveX(ctx)
	exist = pedro.QueryOwner().ExistX(ctx)
	require.False(t, exist)

	require.Equal(t, client.User.Query().CountX(ctx), len(client.User.Query().AllX(ctx)))
	require.Equal(t, client.Pet.Query().CountX(ctx), len(client.Pet.Query().AllX(ctx)))

	nat := client.User.Create().SetName("nati").AddFriends(a8m).SaveX(ctx)
	users := client.User.Query().WithFriends().WithFriendships().WithGroups().Order(ent.Asc(user.FieldName)).AllX(ctx)
	require.Len(t, users, 2)
	require.Equal(t, users[0].Name, a8m.Name)
	require.Equal(t, users[1].Name, nat.Name)
	require.Len(t, users[0].Edges.Groups, 1)
	require.Len(t, users[1].Edges.Groups, 0)
	require.Len(t, users[0].Edges.Friends, 1)
	require.Len(t, users[1].Edges.Friends, 1)
	require.Len(t, users[0].Edges.Friendships, 1)
	require.Len(t, users[1].Edges.Friendships, 1)

	ta := client.User.Create().SetName("ta").AddParents(a8m, nat).SaveX(ctx)
	el := client.User.Create().SetName("el").AddParents(a8m, nat).SaveX(ctx)
	jo := client.User.Create().SetName("be").AddParents(a8m, nat).SaveX(ctx)

	require.Equal(t, 3, client.User.Query().Where(user.HasParents()).CountX(ctx))
	require.Equal(t, 3, a8m.QueryChildren().CountX(ctx))

	sib := ta.QueryParents().QueryChildren().Where(user.NameNEQ(ta.Name)).AllX(ctx)
	require.Len(t, sib, 2)
	require.True(t, slices.ContainsFunc(sib, func(u *ent.User) bool { return u.Name == el.Name }))
	require.True(t, slices.ContainsFunc(sib, func(u *ent.User) bool { return u.Name == jo.Name }))

	// Cross-schema edge predicate: QueryGroups().Where(HasUsersWith(...))
	// must qualify group_users with db2, since the connection defaults to db1.
	got := a8m.QueryGroups().
		Where(group.HasUsersWith(user.Id(a8m.Id))).
		CountX(ctx)
	require.Equal(t, 1, got) // a8m was removed from GitHub above; only GitLab remains
}

// TestSchemaConfigFromAnnotations is the other half of what TestMySql covers.
//
// There are two ways a client learns which schema a table sits in. TestMySql
// uses the one passed at runtime -- ent.AlternateSchema(cfg) -- and this uses
// the one written down at codegen: the `versioned` package declares
// entsql.Schema on its own schemas, so the mapping arrives as
// versioned.DefaultSchemaConfig and no caller has to say it. Everything below
// the setup is the same assertions as TestMySql, run through that client.
//
// The databases are built with ent's own DDL rather than by the Atlas CLI,
// which is what this test used to do. The CLI cannot read this fork's schemas
// at all -- `atlas migrate diff --to ent://...` shells out to
// `entgo.io/ent/cmd/ent`, a module path that is not ours -- so the checked-in
// migration directory it applied could not be regenerated after the rename
// pass, and had gone stale in four ways at once: `pets` for `pet`, `friend_id`
// for `friends_id`, `friendship_user_id_friend_id` for
// `friendship_user_id_friends_id`, `pets_users_pets` for `pet_user_pets`. It
// was invisible because the test skipped everywhere.
//
// What ent does with a versioned migration -- planning, applying and recording
// revisions, with no CLI in it -- is dialect/sql/schema/versioned.go, covered
// by dialect/sql/schema/integration and by Versioned in entc/integration/migrate.
func TestSchemaConfigFromAnnotations(t *testing.T) {
	db, err := sql.Open("mysql", "root:pass@tcp(localhost:3308)/?parseTime=true&multiStatements=true")
	require.NoError(t, err)
	ctx := context.Background()
	t.Cleanup(func() {
		db.ExecContext(ctx, "SET foreign_key_checks = 0")
		for _, name := range []string{"db1", "db2", "db3"} {
			db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", name))
		}
		db.ExecContext(ctx, "SET foreign_key_checks = 1")
		db.Close()
	})

	// The generated migrate tables carry no schema of their own -- the
	// annotation reaches the client, not the DDL -- so the placement is taken
	// from the client's own answer rather than written a second time here.
	cfg := versioned.DefaultSchemaConfig
	for tbl, name := range map[*schema.Table]string{
		vmigrate.FriendshipTable:    cfg.Friendship,
		vmigrate.GroupTable:         cfg.Group,
		vmigrate.GroupUsersTable:    cfg.GroupUsers,
		vmigrate.PetTable:           cfg.Pet,
		vmigrate.UserTable:          cfg.User,
		vmigrate.UserFollowingTable: cfg.UserFollowing,
	} {
		require.NotEmpty(t, name, "table %q has no schema in DefaultSchemaConfig", tbl.Name)
		tbl.Schema = name
	}
	pl, err := schema.Dump(ctx, dialect.MySql, "8.0.19", vmigrate.Tables)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, pl)
	require.NoError(t, err)

	// No default database on the connection: every table is qualified by the
	// config, which is the point.
	client, err := versioned.Open("mysql", "root:pass@tcp(localhost:3308)/?parseTime=true")
	require.NoError(t, err)
	defer client.Close()

	// Copy of the test above.
	pedro := client.Pet.Create().SetName("Pedro").SaveX(ctx)
	groups := client.Group.CreateBulk(
		client.Group.Create().SetName("GitHub"),
		client.Group.Create().SetName("GitLab"),
	).SaveX(ctx)
	a8m := client.User.Create().SetName("a8m").AddPets(pedro).AddGroups(groups...).SaveX(ctx)

	// Custom modifier with schema config.
	var names []struct {
		User string `sql:"user_name"`
		Pet  string `sql:"pet_name"`
	}
	client.Pet.Query().
		Modify(func(s *sql.Selector) {
			// The below function is exported using a custom
			// template defined in ent/template/config.tmpl.
			cfg := versioned.DefaultSchemaConfig
			t := sql.Table(user.Table).Schema(cfg.User)
			s.Join(t).On(s.C(pet.FieldOwnerId), t.C(user.FieldId))
			s.Select(
				sql.As(t.C(user.FieldName), "user_name"),
				sql.As(s.C(pet.FieldName), "pet_name"),
			)
		}).
		ScanX(ctx, &names)
	require.Len(t, names, 1)
	require.Equal(t, "a8m", names[0].User)
	require.Equal(t, "Pedro", names[0].Pet)

	id := client.Group.Query().
		Where(vgroup.HasUsersWith(vuser.Id(a8m.Id))).
		Limit(1).
		QueryUsers().
		QueryPets().
		OnlyIdX(ctx)
	require.Equal(t, pedro.Id, id)

	affected := client.Group.
		Update().
		ClearUsers().
		Where(
			vgroup.And(
				vgroup.Name(groups[0].Name),
				vgroup.HasUsersWith(
					vuser.HasPetsWith(
						vpet.Name(pedro.Name),
					),
				),
			),
		).
		SaveX(ctx)
	require.Equal(t, 1, affected)

	exist := groups[0].QueryUsers().ExistX(ctx)
	require.False(t, exist)
	exist = groups[1].QueryUsers().ExistX(ctx)
	require.True(t, exist)
	exist = pedro.QueryOwner().ExistX(ctx)
	require.True(t, exist)
	pedro = pedro.Update().ClearOwner().SaveX(ctx)
	exist = pedro.QueryOwner().ExistX(ctx)
	require.False(t, exist)

	require.Equal(t, client.User.Query().CountX(ctx), len(client.User.Query().AllX(ctx)))
	require.Equal(t, client.Pet.Query().CountX(ctx), len(client.Pet.Query().AllX(ctx)))

	nat := client.User.Create().SetName("nati").AddFriends(a8m).SaveX(ctx)
	users := client.User.Query().WithFriends().WithFriendships().WithGroups().Order(ent.Asc(user.FieldName)).AllX(ctx)
	require.Len(t, users, 2)
	require.Equal(t, users[0].Name, a8m.Name)
	require.Equal(t, users[1].Name, nat.Name)
	require.Len(t, users[0].Edges.Groups, 1)
	require.Len(t, users[1].Edges.Groups, 0)
	require.Len(t, users[0].Edges.Friends, 1)
	require.Len(t, users[1].Edges.Friends, 1)
	require.Len(t, users[0].Edges.Friendships, 1)
	require.Len(t, users[1].Edges.Friendships, 1)
}
