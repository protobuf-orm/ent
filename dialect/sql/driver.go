// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/protobuf-orm/ent/dialect"
)

// Driver is a dialect.Driver implementation for Sql based databases.
type Driver struct {
	Conn
	dialect string
}

// NewDriver creates a new Driver with the given Conn and dialect.
func NewDriver(dialect string, c Conn) *Driver {
	return &Driver{dialect: dialect, Conn: c}
}

// Open wraps the database/sql.Open method and returns a dialect.Driver that implements the an ent/dialect.Driver interface.
func Open(dialect, source string) (*Driver, error) {
	db, err := sql.Open(dialect, source)
	if err != nil {
		return nil, err
	}
	return NewDriver(dialect, Conn{db, dialect}), nil
}

// OpenDB wraps the given database/sql.DB method with a Driver.
func OpenDB(dialect string, db *sql.DB) *Driver {
	return NewDriver(dialect, Conn{db, dialect})
}

// DB returns the underlying *sql.DB instance.
func (d Driver) DB() *sql.DB {
	return d.ExecQuerier.(*sql.DB)
}

// Dialect implements the dialect.Dialect method.
func (d Driver) Dialect() string {
	// If the underlying driver is wrapped with a telemetry driver.
	for _, name := range []string{dialect.MySql, dialect.SQLite, dialect.Postgres} {
		if strings.HasPrefix(d.dialect, name) {
			return name
		}
	}
	return d.dialect
}

// Tx starts and returns a transaction.
func (d *Driver) Tx(ctx context.Context) (dialect.Tx, error) {
	return d.BeginTx(ctx, nil)
}

// BeginTx starts a transaction with options.
func (d *Driver) BeginTx(ctx context.Context, opts *TxOptions) (dialect.Tx, error) {
	tx, err := d.DB().BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{
		Conn: Conn{tx, d.dialect},
		Tx:   tx,
	}, nil
}

// Close closes the underlying connection.
func (d *Driver) Close() error { return d.DB().Close() }

// Tx implements dialect.Tx interface.
type Tx struct {
	Conn
	driver.Tx
}

// ctyVarsKey is the key used for attaching and reading the context variables.
type ctxVarsKey struct{}

// sessionVars holds sessions/transactions variables to set before every statement.
type sessionVars struct {
	vars []struct{ k, v string }
}

// WithVar returns a new context that holds the session variable to be executed before every query.
func WithVar(ctx context.Context, name, value string) context.Context {
	sv, _ := ctx.Value(ctxVarsKey{}).(sessionVars)
	sv.vars = append(sv.vars, struct {
		k, v string
	}{
		k: name,
		v: value,
	})
	return context.WithValue(ctx, ctxVarsKey{}, sv)
}

// VarFromContext returns the session variable value from the context.
func VarFromContext(ctx context.Context, name string) (string, bool) {
	sv, _ := ctx.Value(ctxVarsKey{}).(sessionVars)
	for _, s := range sv.vars {
		if s.k == name {
			return s.v, true
		}
	}
	return "", false
}

// WithIntVar calls WithVar with the string representation of the value.
func WithIntVar(ctx context.Context, name string, value int) context.Context {
	return WithVar(ctx, name, strconv.Itoa(value))
}

// ExecQuerier wraps the standard Exec and Query methods.
type ExecQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Conn implements dialect.ExecQuerier given ExecQuerier.
type Conn struct {
	ExecQuerier
	dialect string
}

// bindArgs converts the arguments database/sql would convert itself but a
// driver with a converter of its own will not.
//
// A driver that implements driver.NamedValueChecker is asked before
// database/sql's default converter, and go-sql-driver/mysql's answers
// "unsupported type uuid.UUID, a array". So the same schema that stores a UUID
// on SQLite, where the default converter is reached, fails to write one on
// MySQL. Converting here makes the two agree, and agree on the canonical text
// -- which is what the default converter produces, and what
// github.com/google/uuid's driver.Valuer produced before the standard library
// had the type.
//
// It is the standard library's own short list and not a rule about types: a
// type of one's own that is sixteen bytes with a MarshalText, which is exactly
// what a uuid.UUID is, is refused by every one of these converters and is
// meant to be. See field.RType.DriverType.
//
// # Why a time is moved to UTC
//
// For the same reason and by a different mechanism. A time.Time carries the
// zone it was read in, and what a column does with that zone is the dialect's
// own business: a Postgres timestamptz and a MySQL timestamp parse the offset
// and normalise, while a SQLite datetime is text and keeps whatever it was
// handed. So one instant written by a process in KST and by one in a container
// with no TZ set is two rows on SQLite that are equal as instants and equal to
// nothing else -- not to `=`, not ordered by ORDER BY, not found by the `>` a
// keyset cursor is built from. See dialect/sql/sqlpage.
//
// Moving to UTC discards no zone in any sense that matters. The instant is
// untouched and the offset is still stated, as `Z`; what goes is the freedom
// to state it more than one way. It also drops the monotonic reading time.Now
// attaches, which means nothing in a column and nothing across processes.
//
// What it does not reach: a time inside a JSON value, which encoding/json has
// already written with its offset by the time it arrives here, and anything
// executed through Driver.DB rather than through this package.
func bindArgs(args []any) []any {
	var out []any
	for i, v := range args {
		// A pointer is dereferenced rather than rewritten, which is what
		// database/sql's own converter does with one.
		var b any
		switch v := v.(type) {
		case uuid.UUID:
			b = v.String()
		case *uuid.UUID:
			if v == nil {
				continue
			}
			b = v.String()
		case time.Time:
			b = v.UTC()
		case *time.Time:
			if v == nil {
				continue
			}
			b = v.UTC()
		case sql.NullTime:
			if !v.Valid {
				continue
			}
			b = sql.NullTime{Time: v.Time.UTC(), Valid: true}
		case *sql.NullTime:
			if v == nil || !v.Valid {
				continue
			}
			b = sql.NullTime{Time: v.Time.UTC(), Valid: true}
		default:
			continue
		}
		if out == nil {
			out = make([]any, len(args))
			copy(out, args)
		}
		out[i] = b
	}
	if out == nil {
		return args
	}
	return out
}

// Exec implements the dialect.Exec method.
func (c Conn) Exec(ctx context.Context, query string, args, v any) (rerr error) {
	argv, ok := args.([]any)
	if !ok {
		return fmt.Errorf("dialect/sql: invalid type %T. expect []any for args", v)
	}
	ex, cf, err := c.maySetVars(ctx)
	if err != nil {
		return err
	}
	if cf != nil {
		defer func() { rerr = errors.Join(rerr, cf()) }()
	}
	switch v := v.(type) {
	case nil:
		if _, err := ex.ExecContext(ctx, query, bindArgs(argv)...); err != nil {
			return err
		}
	case *sql.Result:
		res, err := ex.ExecContext(ctx, query, bindArgs(argv)...)
		if err != nil {
			return err
		}
		*v = res
	default:
		return fmt.Errorf("dialect/sql: invalid type %T. expect *sql.Result", v)
	}
	return nil
}

// Query implements the dialect.Query method.
func (c Conn) Query(ctx context.Context, query string, args, v any) error {
	vr, ok := v.(*Rows)
	if !ok {
		return fmt.Errorf("dialect/sql: invalid type %T. expect *sql.Rows", v)
	}
	argv, ok := args.([]any)
	if !ok {
		return fmt.Errorf("dialect/sql: invalid type %T. expect []any for args", args)
	}
	ex, cf, err := c.maySetVars(ctx)
	if err != nil {
		return err
	}
	rows, err := ex.QueryContext(ctx, query, bindArgs(argv)...)
	if err != nil {
		if cf != nil {
			err = errors.Join(err, cf())
		}
		return err
	}
	*vr = Rows{rows}
	if cf != nil {
		vr.ColumnScanner = rowsWithCloser{rows, cf}
	}
	return nil
}

// mysqlUnset is what puts a MySql variable back the way it was found.
//
// The two kinds of variable disagree about it, and each refuses the other's
// answer. A system variable goes back with DEFAULT, which is its global value;
// NULL is refused outright -- `SET time_zone = NULL` is error 1231, "Variable
// 'time_zone' can't be set to the value of 'NULL'". A user-defined variable,
// the ones written with a single `@`, has no global value to go back to and is
// unset with NULL; DEFAULT is a syntax error there. MySql 8.0 and 8.4 and
// MariaDB 10.11 and 11.4 all answer the same way.
//
// Getting it wrong fails *after* the query rather than instead of it. The
// reset runs on the way back to the pool, so the statement the caller asked
// for succeeds and the error surfaces from somewhere the caller never named.
func mysqlUnset(name string) string {
	// `@@x` is a system variable spelled the long way; only a single `@` makes
	// it one of the caller's own.
	if strings.HasPrefix(name, "@") && !strings.HasPrefix(name, "@@") {
		return "NULL"
	}
	return "DEFAULT"
}

// maySetVars sets the session variables before executing a query.
func (c Conn) maySetVars(ctx context.Context) (ExecQuerier, func() error, error) {
	sv, _ := ctx.Value(ctxVarsKey{}).(sessionVars)
	if len(sv.vars) == 0 {
		return c, nil, nil
	}
	var (
		ex    ExecQuerier  // Underlying ExecQuerier.
		cf    func() error // Close function.
		reset []string     // Reset variables.
		seen  = make(map[string]struct{}, len(sv.vars))
	)
	switch e := c.ExecQuerier.(type) {
	case *sql.Tx:
		ex = e
	case *sql.DB:
		conn, err := e.Conn(ctx)
		if err != nil {
			return nil, nil, err
		}
		ex, cf = conn, conn.Close
	}
	for _, s := range sv.vars {
		if _, ok := seen[s.k]; !ok {
			switch c.dialect {
			case dialect.Postgres:
				reset = append(reset, fmt.Sprintf("RESET %s", s.k))
			case dialect.MySql:
				reset = append(reset, fmt.Sprintf("SET %s = %s", s.k, mysqlUnset(s.k)))
			}
			seen[s.k] = struct{}{}
		}
		if _, err := ex.ExecContext(ctx, fmt.Sprintf("SET %s = '%s'", s.k, s.v)); err != nil {
			if cf != nil {
				err = errors.Join(err, cf())
			}
			return nil, nil, err
		}
	}
	// If there are variables to reset, and we need to return the
	// connection to the pool, we need to clean up the variables.
	if cls := cf; cf != nil && len(reset) > 0 {
		cf = func() error {
			for _, q := range reset {
				if _, err := ex.ExecContext(ctx, q); err != nil {
					return errors.Join(err, cls())
				}
			}
			return cls()
		}
	}
	return ex, cf, nil
}

var _ dialect.Driver = (*Driver)(nil)

type (
	// Rows wraps the sql.Rows to avoid locks copy.
	Rows struct{ ColumnScanner }
	// Result is an alias to sql.Result.
	Result = sql.Result
	// NullBool is an alias to sql.NullBool.
	NullBool = sql.NullBool
	// NullInt64 is an alias to sql.NullInt64.
	NullInt64 = sql.NullInt64
	// NullString is an alias to sql.NullString.
	NullString = sql.NullString
	// NullFloat64 is an alias to sql.NullFloat64.
	NullFloat64 = sql.NullFloat64
	// NullTime represents a time.Time that may be null.
	NullTime = sql.NullTime
	// TxOptions holds the transaction options to be used in DB.BeginTx.
	TxOptions = sql.TxOptions
)

// Scan reads a row into dest, and answers a time in UTC.
//
// It is the other half of what bindArgs does to an argument, and it is here
// for the same reason: the zone is the dialect's answer rather than the
// column's. A Postgres timestamptz comes back in whatever the session's
// TimeZone is, as a zone with no name; MySql's comes back in the connection's
// `loc`; SQLite's comes back with whatever offset the text carried. The
// instant is right in all three and the [time.Time.Location] is three
// different answers, so a value read on one engine is not the value read on
// another -- `==`, reflect.DeepEqual and every test helper built on them say
// so, while [time.Time.Equal] says otherwise.
//
// This method exists rather than a change to the generated scan code because
// it is one place. Rows embeds the interface it scans through, so naming Scan
// here shadows the promoted one for every caller that holds a Rows: the node
// query in dialect/sql/sqlgraph, and ScanOne and ScanSlice below.
//
// What it cannot reach is a destination that scans itself -- a field.GoType
// with a Scan method of its own gets the driver's value and decides. That is
// the same hole a custom driver.Valuer leaves on the way out.
func (r Rows) Scan(dest ...any) error {
	if err := r.ColumnScanner.Scan(dest...); err != nil {
		return err
	}
	for _, d := range dest {
		switch d := d.(type) {
		case *time.Time:
			*d = d.UTC()
		case *sql.NullTime:
			if d.Valid {
				d.Time = d.Time.UTC()
			}
		case *any:
			// What ScanSlice reads a column of unknown type through.
			if t, ok := (*d).(time.Time); ok {
				*d = t.UTC()
			}
		}
	}
	return nil
}

// NullScanner implements the sql.Scanner interface such that it
// can be used as a scan destination, similar to the types above.
type NullScanner struct {
	S     sql.Scanner
	Valid bool // Valid is true if the Scan value is not NULL.
}

// Scan implements the Scanner interface.
func (n *NullScanner) Scan(value any) error {
	n.Valid = value != nil
	if n.Valid {
		return n.S.Scan(value)
	}
	return nil
}

// Null is database/sql.Null, re-exported so that generated code can name it
// with the same import it names the rest of this package by.
//
// It is what a column that may hold NULL is scanned through when the Go type
// has no Scan of its own to notice one -- a uuid.UUID, say, which database/sql
// reads and writes itself but which cannot represent an absence.
type Null[T any] = sql.Null[T]

// ColumnScanner is the interface that wraps the standard
// sql.Rows methods used for scanning database rows.
type ColumnScanner interface {
	Close() error
	ColumnTypes() ([]*sql.ColumnType, error)
	Columns() ([]string, error)
	Err() error
	Next() bool
	NextResultSet() bool
	Scan(dest ...any) error
}

// rowsWithCloser wraps the ColumnScanner interface with a custom Close hook.
type rowsWithCloser struct {
	ColumnScanner
	closer func() error
}

// Close closes the underlying ColumnScanner and calls the custom closer.
func (r rowsWithCloser) Close() error {
	err := r.ColumnScanner.Close()
	return errors.Join(err, r.closer())
}
