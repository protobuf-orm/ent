// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package sql

import (
	"context"
	"testing"
	"time"

	"github.com/protobuf-orm/ent/dialect"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestWithVars(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	drv := OpenDB(dialect.Postgres, db)
	mock.ExpectExec("SET foo = 'bar'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectExec("RESET foo").WillReturnResult(sqlmock.NewResult(0, 0))
	rows := &Rows{}
	err = drv.Query(
		WithVar(context.Background(), "foo", "bar"),
		"SELECT 1",
		[]any{},
		rows,
	)
	require.NoError(t, err)
	require.NoError(t, rows.Close(), "rows should be closed to release the connection")
	require.NoError(t, mock.ExpectationsWereMet())

	mock.ExpectExec("SET foo = 'bar'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SET foo = 'baz'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectExec("RESET foo").WillReturnResult(sqlmock.NewResult(0, 0))
	err = drv.Query(
		WithVar(WithVar(context.Background(), "foo", "bar"), "foo", "baz"),
		"SELECT 1",
		[]any{},
		rows,
	)
	require.NoError(t, err)
	require.NoError(t, rows.Close(), "rows should be closed to release the connection")
	require.NoError(t, mock.ExpectationsWereMet())

	mock.ExpectBegin()
	mock.ExpectExec("SET foo = 'bar'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectCommit()
	tx, err := drv.Tx(context.Background())
	require.NoError(t, err)
	err = tx.Query(
		WithVar(context.Background(), "foo", "bar"),
		"SELECT 1",
		[]any{},
		rows,
	)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
	// Rows should not be closed to release the session,
	// as a transaction is always scoped to a single connection.

	mock.ExpectExec("SET foo = 'qux'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO users DEFAULT VALUES").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("RESET foo").WillReturnResult(sqlmock.NewResult(0, 0))
	err = drv.Exec(
		WithVar(context.Background(), "foo", "qux"),
		"INSERT INTO users DEFAULT VALUES",
		[]any{},
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	// No rows are returned, so no need to close them.

	mock.ExpectExec("SET foo = 'foo'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO users DEFAULT VALUES").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("RESET foo").WillReturnResult(sqlmock.NewResult(0, 0))
	err = drv.Exec(
		WithVar(context.Background(), "foo", "foo"),
		"INSERT INTO users DEFAULT VALUES",
		[]any{},
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	// No rows are returned, so no need to close them.
}

// TestBindArgsTime is the whole of what dialect/sql promises about a time it is
// handed: the instant survives and the way it is written does not vary.
func TestBindArgsTime(t *testing.T) {
	at := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	kst := time.FixedZone("KST", 9*60*60)
	local := at.In(kst)

	nt := NullTime{Time: local, Valid: true}
	null := NullTime{}
	var nilp *time.Time

	got := bindArgs([]any{local, &local, nt, &nt, null, nilp, "unchanged"})

	// The instant is what is written; the zone it was written in is not.
	require.Equal(t, at, got[0], "a time is bound as UTC")
	require.Equal(t, at, got[1], "a pointer to a time is dereferenced, as a *uuid.UUID is")
	require.Equal(t, NullTime{Time: at, Valid: true}, got[2])
	require.Equal(t, NullTime{Time: at, Valid: true}, got[3])

	// Nothing else is touched, and an absence stays an absence.
	require.Equal(t, null, got[4], "a NULL has no zone to move")
	require.Nil(t, got[5], "a nil is left as it was")
	require.Equal(t, "unchanged", got[6])

	// It is the same instant, not merely a comparable one.
	require.True(t, local.Equal(got[0].(time.Time)))

	// And the monotonic reading time.Now attaches is gone, which is what makes
	// a bound time comparable to one that was read back.
	now := bindArgs([]any{time.Now()})[0].(time.Time)
	require.Equal(t, now, now.Round(0), "the monotonic reading is dropped")
}

// TestBindArgsUnchanged keeps the fast path honest: args with nothing to
// convert are handed on as they came, not copied.
func TestBindArgsUnchanged(t *testing.T) {
	args := []any{1, "a", nil}
	require.Equal(t, args, bindArgs(args))
}

// TestRowsScanTime covers the destinations the engine tests cannot reach: a
// NullTime, and the `any` a column of unknown type is read through.
func TestRowsScanTime(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	at := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)
	local := at.In(time.FixedZone("KST", 9*60*60))
	mock.ExpectQuery("SELECT").WillReturnRows(
		sqlmock.NewRows([]string{"a", "b", "c", "d"}).AddRow(local, local, local, nil),
	)

	rows := &Rows{}
	require.NoError(t, OpenDB(dialect.SQLite, db).Query(context.Background(), "SELECT", []any{}, rows))
	defer rows.Close()
	require.True(t, rows.Next())

	var (
		plain time.Time
		valid NullTime
		blank NullTime
		nul   any
	)
	require.NoError(t, rows.Scan(&plain, &valid, &nul, &blank))
	require.Equal(t, at, plain)
	require.Equal(t, NullTime{Time: at, Valid: true}, valid)
	require.Equal(t, at, nul)
	require.Equal(t, NullTime{}, blank, "a NULL has no zone to move")
}

func TestMySqlUnset(t *testing.T) {
	for name, want := range map[string]string{
		"time_zone":           "DEFAULT",
		"sql_mode":            "DEFAULT",
		"@@SESSION.time_zone": "DEFAULT",
		"@@time_zone":         "DEFAULT",
		"@x":                  "NULL",
		"@my_var":             "NULL",
	} {
		require.Equalf(t, want, mysqlUnset(name), "SET %s = ?", name)
	}
}

// TestWithVarsMySql pins the statements a MySql session variable is set and
// unset with, which the engine tests in entc/integration/entvar check an
// engine agrees to.
func TestWithVarsMySql(t *testing.T) {
	for name, unset := range map[string]string{"time_zone": "DEFAULT", "@x": "NULL"} {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			db.SetMaxOpenConns(1)

			mock.ExpectExec("SET " + name + " = 'v'").WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
			mock.ExpectExec("SET " + name + " = " + unset).WillReturnResult(sqlmock.NewResult(0, 0))

			rows := &Rows{}
			require.NoError(t, OpenDB(dialect.MySql, db).Query(
				WithVar(context.Background(), name, "v"), "SELECT 1", []any{}, rows,
			))
			require.NoError(t, rows.Close())
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
