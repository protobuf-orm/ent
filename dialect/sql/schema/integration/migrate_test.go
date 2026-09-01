// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package integration holds the tests of github.com/protobuf-orm/ent/dialect/sql/schema that
// run against a real database engine. They live in a module of their own so
// that the driver they need stays out of the go.mod of ent itself.
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"text/template"
	"time"

	"ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/schema"
	"ariga.io/atlas/sql/sqlite"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/entsql"
	"github.com/protobuf-orm/ent/dialect/sql"
	entschema "github.com/protobuf-orm/ent/dialect/sql/schema"
	"github.com/protobuf-orm/ent/schema/field"

	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestMigrate_DiffJoinTableAllocationBC(t *testing.T) {
	// Due to a bug in previous versions, if the universal Id option was enabled and the schema did contain an M2M
	// relation, the join table would have had an entry for the join table in the types table. This test ensures,
	// that the PK range allocated for the join table stays in place, since it's removal would break existing projects
	// due to shifted ranges.

	db, err := sql.Open(dialect.SQLite, "file:test?mode=memory&_fk=1")
	require.NoError(t, err)

	// Mock an existing database with an allocation for a join table.
	for _, stmt := range []string{
		"CREATE TABLE `groups` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL);",
		"CREATE INDEX `short` ON `groups` (`id`);",
		"CREATE INDEX `long____________________________1cb2e7e47a309191385af4ad320875b1` ON `groups` (`id`);",
		"CREATE TABLE `users` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL);",
		"INSERT INTO sqlite_sequence (name, seq) VALUES ('users', 4294967296);",
		"CREATE TABLE `user_groups` (`user_id` integer NOT NULL, `group_id` integer NOT NULL, PRIMARY KEY (`user_id`, `group_id`), CONSTRAINT `user_groups_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE, CONSTRAINT `user_groups_group_id` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE CASCADE);",
		"INSERT INTO sqlite_sequence (name, seq) VALUES ('user_groups', 8589934592);",
		"CREATE TABLE `ent_types` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `type` text NOT NULL);",
		"CREATE UNIQUE INDEX `ent_types_type_key` ON `ent_types` (`type`);",
		"INSERT INTO `ent_types` (`type`) VALUES ('groups'), ('users'), ('user_groups');",
		"INSERT INTO `groups` (`name`) VALUES ('seniors'), ('juniors')",
		"INSERT INTO `users` (`name`) VALUES ('masseelch'), ('a8m'), ('rotemtam')",
		"INSERT INTO `user_groups` (`user_id`, `group_id`) VALUES (4294967297, 1), (4294967298, 1), (4294967299, 2)",
	} {
		_, err := db.ExecContext(context.Background(), stmt)
		require.NoError(t, err)
	}

	// Expect to have no changes when migration runs with fix.
	m, err := entschema.NewMigrate(db, entschema.WithGlobalUniqueId(true), entschema.WithDiffHook(func(next entschema.Differ) entschema.Differ {
		return entschema.DiffFunc(func(current, desired *schema.Schema) ([]schema.Change, error) {
			changes, err := next.Diff(current, desired)
			if err != nil {
				return nil, err
			}
			require.Len(t, changes, 0)
			return changes, nil
		})
	}))
	require.NoError(t, err)
	require.NoError(t, m.Create(context.Background(), tables...))

	// Expect to have no changes to the allocation when the join table is dropped.
	m, err = entschema.NewMigrate(db, entschema.WithGlobalUniqueId(true))
	require.NoError(t, err)
	require.NoError(t, m.Create(context.Background(), groupsTable, usersTable))

	rows, err := db.QueryContext(context.Background(), "SELECT `type` from `ent_types` ORDER BY `id` ASC")
	require.NoError(t, err)
	var types []string
	for rows.Next() {
		var typ string
		require.NoError(t, rows.Scan(&typ))
		types = append(types, typ)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []string{"groups", "users", "user_groups"}, types)
}

var (
	groupsColumns = []*entschema.Column{
		{Name: "id", Type: field.TypeInt, Increment: true},
		{Name: "name", Type: field.TypeString},
	}
	groupsTable = &entschema.Table{
		Name:       "groups",
		Columns:    groupsColumns,
		PrimaryKey: []*entschema.Column{groupsColumns[0]},
		Indexes: []*entschema.Index{
			{
				Name:    "short",
				Columns: []*entschema.Column{groupsColumns[0]}},
			{
				Name:    "long_" + strings.Repeat("_", 60),
				Columns: []*entschema.Column{groupsColumns[0]},
			},
		},
	}
	usersColumns = []*entschema.Column{
		{Name: "id", Type: field.TypeInt, Increment: true},
		{Name: "name", Type: field.TypeString},
	}
	usersTable = &entschema.Table{
		Name:       "users",
		Columns:    usersColumns,
		PrimaryKey: []*entschema.Column{usersColumns[0]},
	}
	userGroupsColumns = []*entschema.Column{
		{Name: "user_id", Type: field.TypeInt},
		{Name: "group_id", Type: field.TypeInt},
	}
	userGroupsTable = &entschema.Table{
		Name:       "user_groups",
		Columns:    userGroupsColumns,
		PrimaryKey: []*entschema.Column{userGroupsColumns[0], userGroupsColumns[1]},
		ForeignKeys: []*entschema.ForeignKey{
			{
				Symbol:     "user_groups_user_id",
				Columns:    []*entschema.Column{userGroupsColumns[0]},
				RefColumns: []*entschema.Column{usersColumns[0]},
				OnDelete:   entschema.Cascade,
			},
			{
				Symbol:     "user_groups_group_id",
				Columns:    []*entschema.Column{userGroupsColumns[1]},
				RefColumns: []*entschema.Column{groupsColumns[0]},
				OnDelete:   entschema.Cascade,
			},
		},
	}
	tables = []*entschema.Table{
		groupsTable,
		usersTable,
		userGroupsTable,
	}
	petColumns = []*entschema.Column{
		{Name: "id", Type: field.TypeInt, Increment: true},
	}
	petsTable = &entschema.Table{
		Name:       "pets",
		Columns:    petColumns,
		PrimaryKey: petColumns,
	}
)

func init() {
	userGroupsTable.ForeignKeys[0].RefTable = usersTable
	userGroupsTable.ForeignKeys[1].RefTable = groupsTable
}

func TestMigrate_Diff(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open(dialect.SQLite, "file:test?mode=memory&_fk=1")
	require.NoError(t, err)

	p := t.TempDir()
	d, err := migrate.NewLocalDir(p)
	require.NoError(t, err)

	m, err := entschema.NewMigrate(db, entschema.WithDir(d))
	require.NoError(t, err)
	require.NoError(t, m.Diff(ctx, &entschema.Table{Name: "users"}))
	v := time.Now().UTC().Format("20060102150405")
	requireFileEqual(t, filepath.Join(p, v+"_changes.up.sql"), "-- create \"users\" table\nCREATE TABLE `users` ();\n")
	requireFileEqual(t, filepath.Join(p, v+"_changes.down.sql"), "-- reverse: create \"users\" table\nDROP TABLE `users`;\n")
	require.FileExists(t, filepath.Join(p, migrate.HashFileName))

	// Test integrity file.
	p = t.TempDir()
	d, err = migrate.NewLocalDir(p)
	require.NoError(t, err)
	m, err = entschema.NewMigrate(db, entschema.WithDir(d))
	require.NoError(t, err)
	require.NoError(t, m.Diff(ctx, &entschema.Table{Name: "users"}))
	requireFileEqual(t, filepath.Join(p, v+"_changes.up.sql"), "-- create \"users\" table\nCREATE TABLE `users` ();\n")
	requireFileEqual(t, filepath.Join(p, v+"_changes.down.sql"), "-- reverse: create \"users\" table\nDROP TABLE `users`;\n")
	require.FileExists(t, filepath.Join(p, migrate.HashFileName))
	require.NoError(t, d.WriteFile("tmp.sql", nil))
	require.ErrorIs(t, m.Diff(ctx, &entschema.Table{Name: "users"}), migrate.ErrChecksumMismatch)

	p = t.TempDir()
	d, err = migrate.NewLocalDir(p)
	require.NoError(t, err)
	f, err := migrate.NewTemplateFormatter(
		template.Must(template.New("").Parse("{{ .Name }}.sql")),
		template.Must(template.New("").Parse(
			`{{ range .Changes }}{{ printf "%s;\n" .Cmd }}{{ end }}`,
		)),
	)
	require.NoError(t, err)

	// Join tables (mapping between user and group) will not result in an entry to the types table.
	m, err = entschema.NewMigrate(db, entschema.WithFormatter(f), entschema.WithDir(d), entschema.WithGlobalUniqueId(true))
	require.NoError(t, err)
	require.NoError(t, m.Diff(ctx, tables...))
	changesSql := strings.Join([]string{
		"CREATE TABLE `groups` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL);",
		"CREATE INDEX `short` ON `groups` (`id`);",
		"CREATE INDEX `long____________________________1cb2e7e47a309191385af4ad320875b1` ON `groups` (`id`);",
		"CREATE TABLE `users` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `name` text NOT NULL);",
		fmt.Sprintf("INSERT INTO sqlite_sequence (name, seq) VALUES ('users', %d);", 1<<32),
		"CREATE TABLE `user_groups` (`user_id` integer NOT NULL, `group_id` integer NOT NULL, PRIMARY KEY (`user_id`, `group_id`), CONSTRAINT `user_groups_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE, CONSTRAINT `user_groups_group_id` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE CASCADE);",
		"CREATE TABLE `ent_types` (`id` integer NOT NULL PRIMARY KEY AUTOINCREMENT, `type` text NOT NULL);",
		"CREATE UNIQUE INDEX `ent_types_type_key` ON `ent_types` (`type`);",
		"INSERT INTO `ent_types` (`type`) VALUES ('groups'), ('users');",
		"",
	}, "\n")
	requireFileEqual(t, filepath.Join(p, "changes.sql"), changesSql)

	// Skipping table creation should write only the ent_type insertion.
	m, err = entschema.NewMigrate(db, entschema.WithFormatter(f), entschema.WithDir(d), entschema.WithGlobalUniqueId(true), entschema.WithDiffOptions(schema.DiffSkipChanges(&schema.AddTable{})))
	require.NoError(t, err)
	require.NoError(t, m.Diff(ctx, tables...))
	requireFileEqual(t, filepath.Join(p, "changes.sql"), "INSERT INTO `ent_types` (`type`) VALUES ('groups'), ('users');\n")

	// Enable indentations.
	m, err = entschema.NewMigrate(db, entschema.WithFormatter(f), entschema.WithDir(d), entschema.WithGlobalUniqueId(true), entschema.WithIndent("  "))
	require.NoError(t, err)
	// Adding another node will result in a new entry to the TypeTable (without actually creating it).
	// Applying the plan as generated also covers that it is valid Sql for a driver built with
	// SqlITE_DQS=0, which is what schema.fixSequenceQuoting is there for.
	_, err = db.ExecContext(ctx, changesSql)
	require.NoError(t, err)
	require.NoError(t, m.NamedDiff(ctx, "changes_2", petsTable))
	requireFileEqual(t,
		filepath.Join(p, "changes_2.sql"), strings.Join([]string{
			"CREATE TABLE `pets` (\n  `id` integer NOT NULL PRIMARY KEY AUTOINCREMENT\n);",
			fmt.Sprintf("INSERT INTO sqlite_sequence (name, seq) VALUES ('pets', %d);", 2<<32),
			"INSERT INTO `ent_types` (`type`) VALUES ('pets');", "",
		}, "\n"))

	// Checksum will be updated as well.
	require.NoError(t, migrate.Validate(d))

	require.NoError(t, m.NamedDiff(ctx, "no_changes"), "should not error if entschema.WithErrNoPlan is not set")
	// Enable entschema.WithErrNoPlan.
	m, err = entschema.NewMigrate(db, entschema.WithFormatter(f), entschema.WithDir(d), entschema.WithGlobalUniqueId(true), entschema.WithErrNoPlan(true))
	require.NoError(t, err)
	err = m.NamedDiff(ctx, "no_changes")
	require.ErrorIs(t, err, migrate.ErrNoPlan)
}

func requireFileEqual(t *testing.T, name, contents string) {
	c, err := os.ReadFile(name)
	require.NoError(t, err)
	require.Equal(t, contents, string(c))
}

func TestAtlas_StateReader(t *testing.T) {
	db, err := sql.Open(dialect.SQLite, "file:test?mode=memory&_fk=1")
	require.NoError(t, err)
	m, err := entschema.NewMigrate(db)
	require.NoError(t, err)
	realm, err := m.StateReader(&entschema.Table{
		Name: "users",
		Columns: []*entschema.Column{
			{Name: "id", Type: field.TypeInt64, Increment: true},
			{Name: "name", Type: field.TypeString},
			{Name: "active", Type: field.TypeBool},
		},
		Annotation: &entsql.Annotation{
			IncrementStart: func(i int) *int { return &i }(100),
		},
	}).ReadState(context.Background())
	require.NoError(t, err)
	require.NotNil(t, realm)
	require.Len(t, realm.Schemas, 1)
	require.Len(t, realm.Schemas[0].Tables, 1)
	require.Equal(t, "users", realm.Schemas[0].Tables[0].Name)
	require.Equal(t, []schema.Attr{&sqlite.AutoIncrement{Seq: 100}}, realm.Schemas[0].Tables[0].Attrs)
	require.Equal(t,
		realm.Schemas[0].Tables[0].Columns,
		[]*schema.Column{
			schema.NewIntColumn("id", "integer").
				AddAttrs(&sqlite.AutoIncrement{}),
			schema.NewStringColumn("name", "text"),
			schema.NewBoolColumn("active", "bool"),
		},
	)
}

func TestAtlas_ParallelCreate(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		db, err := sql.Open(dialect.SQLite, fmt.Sprintf("file:test-%d?mode=memory&_fk=1", i))
		require.NoError(t, err)
		m, err := entschema.NewMigrate(db)
		require.NoError(t, err)
		go func() {
			defer wg.Done()
			require.NoError(t, m.Create(context.Background(), petsTable))
			require.NoError(t, db.Close())
		}()
	}
	wg.Wait()
}
