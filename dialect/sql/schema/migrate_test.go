// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"ariga.io/atlas/sql/migrate"
	"ariga.io/atlas/sql/schema"
	"ariga.io/atlas/sql/sqltool"
	"entgo.io/ent/dialect/sql"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestMigrate_SchemaName(t *testing.T) {
	db, mk, err := sqlmock.New()
	require.NoError(t, err)
	mk.ExpectQuery(escape("SHOW server_version_num")).
		WillReturnRows(sqlmock.NewRows([]string{"server_version_num"}).AddRow("130000"))
	mk.ExpectQuery(escape("SELECT current_setting('server_version_num'), current_setting('default_table_access_method', true), current_setting('crdb_version', true)")).
		WillReturnRows(sqlmock.NewRows([]string{"current_setting", "current_setting", "current_setting"}).AddRow("130000", "heap", ""))
	mk.ExpectQuery("SELECT nspname AS schema_name,.+").
		WithArgs("public"). // Schema "public" param is used.
		WillReturnRows(sqlmock.NewRows([]string{"schema_name", "comment"}).AddRow("public", "default schema"))
	mk.ExpectQuery("SELECT t3.oid, t1.table_schema,.+").
		WillReturnRows(sqlmock.NewRows([]string{}))
	m, err := NewMigrate(sql.OpenDB("postgres", db), WithSchemaName("public"), WithDiffHook(func(next Differ) Differ {
		return DiffFunc(func(current, desired *schema.Schema) ([]schema.Change, error) {
			return nil, nil // Noop.
		})
	}))
	require.NoError(t, err)
	require.NoError(t, m.Create(context.Background()))
	require.NoError(t, mk.ExpectationsWereMet())

	// Without schema name the CURRENT_SCHEMA is used.
	mk.ExpectQuery(escape("SHOW server_version_num")).
		WillReturnRows(sqlmock.NewRows([]string{"server_version_num"}).AddRow("130000"))
	mk.ExpectQuery(escape("SELECT current_setting('server_version_num'), current_setting('default_table_access_method', true), current_setting('crdb_version', true)")).
		WillReturnRows(sqlmock.NewRows([]string{"current_setting", "current_setting", "current_setting"}).AddRow("130000", "heap", ""))
	mk.ExpectQuery("SELECT nspname AS schema_name,.+CURRENT_SCHEMA().+").
		WillReturnRows(sqlmock.NewRows([]string{"schema_name", "comment"}).AddRow("public", "default schema"))
	mk.ExpectQuery("SELECT t3.oid, t1.table_schema,.+").
		WillReturnRows(sqlmock.NewRows([]string{}))
	m, err = NewMigrate(sql.OpenDB("postgres", db), WithDiffHook(func(next Differ) Differ {
		return DiffFunc(func(current, desired *schema.Schema) ([]schema.Change, error) {
			return nil, nil // Noop.
		})
	}))
	require.NoError(t, err)
	require.NoError(t, m.Create(context.Background()))
}

func escape(query string) string {
	rows := strings.Split(query, "\n")
	for i := range rows {
		rows[i] = strings.TrimPrefix(rows[i], " ")
	}
	query = strings.Join(rows, " ")
	return strings.TrimSpace(regexp.QuoteMeta(query)) + "$"
}

func TestMigrate_Formatter(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)

	// If no formatter is given it will be set according to the given migration directory implementation.
	for _, tt := range []struct {
		dir migrate.Dir
		fmt migrate.Formatter
	}{
		{&migrate.LocalDir{}, sqltool.GolangMigrateFormatter},
		{&sqltool.GolangMigrateDir{}, sqltool.GolangMigrateFormatter},
		{&sqltool.GooseDir{}, sqltool.GooseFormatter},
		{&sqltool.DBMateDir{}, sqltool.DBMateFormatter},
		{&sqltool.FlywayDir{}, sqltool.FlywayFormatter},
		{&sqltool.LiquibaseDir{}, sqltool.LiquibaseFormatter},
		{struct{ migrate.Dir }{}, sqltool.GolangMigrateFormatter}, // default one if migration dir is unknown
	} {
		m, err := NewMigrate(sql.OpenDB("", db), WithDir(tt.dir))
		require.NoError(t, err)
		require.Equal(t, tt.fmt, m.fmt)
	}

	// If a formatter is given, it is not overridden.
	m, err := NewMigrate(sql.OpenDB("", db), WithDir(&migrate.LocalDir{}), WithFormatter(migrate.DefaultFormatter))
	require.NoError(t, err)
	require.Equal(t, migrate.DefaultFormatter, m.fmt)
}

func TestMigrateWithoutForeignKeys(t *testing.T) {
	tbl := &schema.Table{
		Name: "tbl",
		Columns: []*schema.Column{
			{Name: "id", Type: &schema.ColumnType{Type: &schema.IntegerType{T: "bigint"}}},
		},
	}
	fk := &schema.ForeignKey{
		Symbol:     "fk",
		Table:      tbl,
		Columns:    tbl.Columns[1:],
		RefTable:   tbl,
		RefColumns: tbl.Columns[:1],
		OnUpdate:   schema.NoAction,
		OnDelete:   schema.Cascade,
	}
	tbl.ForeignKeys = append(tbl.ForeignKeys, fk)
	t.Run("AddTable", func(t *testing.T) {
		mdiff := DiffFunc(func(_, _ *schema.Schema) ([]schema.Change, error) {
			return []schema.Change{
				&schema.AddTable{
					T: tbl,
				},
			}, nil
		})
		df, err := withoutForeignKeys(mdiff).Diff(nil, nil)
		require.NoError(t, err)
		require.Len(t, df, 1)
		actual, ok := df[0].(*schema.AddTable)
		require.True(t, ok)
		require.Nil(t, actual.T.ForeignKeys)
	})
	t.Run("ModifyTable", func(t *testing.T) {
		mdiff := DiffFunc(func(_, _ *schema.Schema) ([]schema.Change, error) {
			return []schema.Change{
				&schema.ModifyTable{
					T: tbl,
					Changes: []schema.Change{
						&schema.AddIndex{
							I: &schema.Index{
								Name: "id_key",
								Parts: []*schema.IndexPart{
									{C: tbl.Columns[0]},
								},
							},
						},
						&schema.DropForeignKey{
							F: fk,
						},
						&schema.AddForeignKey{
							F: fk,
						},
						&schema.ModifyForeignKey{
							From:   fk,
							To:     fk,
							Change: schema.ChangeRefColumn,
						},
						&schema.AddColumn{
							C: &schema.Column{Name: "name", Type: &schema.ColumnType{Type: &schema.StringType{T: "varchar(255)"}}},
						},
					},
				},
			}, nil
		})
		df, err := withoutForeignKeys(mdiff).Diff(nil, nil)
		require.NoError(t, err)
		require.Len(t, df, 1)
		actual, ok := df[0].(*schema.ModifyTable)
		require.True(t, ok)
		require.Len(t, actual.Changes, 2)
		addIndex, ok := actual.Changes[0].(*schema.AddIndex)
		require.True(t, ok)
		require.EqualValues(t, "id_key", addIndex.I.Name)
		addColumn, ok := actual.Changes[1].(*schema.AddColumn)
		require.True(t, ok)
		require.EqualValues(t, "name", addColumn.C.Name)
	})
}
