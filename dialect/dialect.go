// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package dialect

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"uuid"
)

// Dialect names for external usage.
const (
	MySql    = "mysql"
	SQLite   = "sqlite3"
	Postgres = "postgres"
)

// ExecQuerier wraps the 2 database operations.
type ExecQuerier interface {
	// Exec executes a query that does not return records. For example, in Sql, INSERT or UPDATE.
	// It scans the result into the pointer v. For SQL drivers, it is dialect/sql.Result.
	Exec(ctx context.Context, query string, args, v any) error
	// Query executes a query that returns rows, typically a SELECT in Sql.
	// It scans the result into the pointer v. For SQL drivers, it is *dialect/sql.Rows.
	Query(ctx context.Context, query string, args, v any) error
}

// Driver is the interface that wraps all necessary operations for ent clients.
type Driver interface {
	ExecQuerier
	// Tx starts and returns a new transaction.
	// The provided context is used until the transaction is committed or rolled back.
	Tx(context.Context) (Tx, error)
	// Close closes the underlying connection.
	Close() error
	// Dialect returns the dialect name of the driver.
	Dialect() string
}

// Tx wraps the Exec and Query operations in transaction.
type Tx interface {
	ExecQuerier
	driver.Tx
}

type nopTx struct {
	Driver
}

func (nopTx) Commit() error   { return nil }
func (nopTx) Rollback() error { return nil }

// NopTx returns a Tx with a no-op Commit / Rollback methods wrapping
// the provided Driver d.
func NopTx(d Driver) Tx {
	return nopTx{d}
}

// InTxDriver is a driver that is a transaction.
//
// It is an interface rather than a type so that the generated txDriver of every
// ent package answers to the same question as the one below: a client asked
// whether it is inside a transaction should say yes to a transaction it did not
// begin itself.
type InTxDriver interface {
	Driver
	// InTx reports that this driver is a transaction. It is always true; what
	// it is for is being asked through a Driver.
	InTx() bool
}

// InTx reports whether drv runs inside a transaction, looking through the
// wrappers a driver picks up on the way.
//
// The wrappers are the reason this is here rather than a type assertion at the
// call site: a driver under a DebugDriver is the same driver, and a caller that
// missed that would start a second transaction inside the first for no reason.
func InTx(drv Driver) bool {
	for {
		switch v := drv.(type) {
		case InTxDriver:
			return v.InTx()
		case *DebugDriver:
			drv = v.Driver
		default:
			return false
		}
	}
}

// BeginTx starts a transaction on drv and answers with a driver that runs
// through it, and with what ends it.
//
// It is how several clients come to share one transaction: build each of them
// on the driver this answers with -- see the WithDriver method of a generated
// client -- and every statement any of them issues is in it.
//
// The transaction is the caller's to commit or roll back, and nobody else's.
// The driver is not it, and what the driver hands out when something asks it
// for a transaction is a NopTx. That is not a nicety: ent ends a transaction
// through whatever Driver.Tx returned, so a driver that returned the real one
// would let any inner call's deferred Rollback throw away the work of the call
// that wrapped it, and report success for it.
//
// What the caller is handed ends the transaction and does nothing else. A
// statement is sent through the driver, which is what asks the transaction's
// [Keep] about it first ([Keep.BeforeStatement]); a transaction that could be
// written through as well would be a second road to the same connection, and
// the one that asks nothing.
//
// The driver is not safe for concurrent use, because a transaction is not.
//
// And it keeps what the transaction holds for the rest of itself -- see
// [Keep] -- which a transaction begun inside another shares with it, the
// same way it shares the transaction. The one this begins does the kept work
// when its caller commits it.
//
// Inside a transaction this did not begin -- one a generated client's
// `Client.Tx` began -- it begins nothing, and answers [ErrForeignTx].
func BeginTx(ctx context.Context, drv Driver) (Driver, driver.Tx, error) {
	// Inside another transaction this one is that one, and so is what it
	// keeps; what is handed back cannot end it, and the outer one's commit
	// does the work. Every statement sent through this one is sent through
	// the outer one too, which is where it is asked about.
	if k, ok := KeepOf(drv); ok {
		tx, err := drv.Tx(ctx)
		if err != nil {
			return nil, nil, err
		}

		return &txDriver{drv: drv, tx: tx, keep: k}, inside{}, nil
	}

	// Inside one that keeps nothing, there is nothing to share and nothing
	// to begin; see [ErrForeignTx].
	if InTx(drv) {
		return nil, nil, ErrForeignTx
	}

	tx, err := drv.Tx(ctx)
	if err != nil {
		return nil, nil, err
	}

	k := &Keep{}
	return &txDriver{drv: drv, tx: tx, keep: k, owns: true}, &keptTx{tx: tx, ctx: ctx, keep: k}, nil
}

// ErrForeignTx is [BeginTx] asked to begin inside a transaction it did not
// begin: one the driver says it is in ([InTx]) and that keeps nothing
// ([KeepOf]), which is a generated client's `Client.Tx`.
//
// Refused rather than begun, because there is no right thing to begin. The
// transaction keeps nothing to share, and one begun inside it keeping its own
// would answer to the wrong end: its commit would write what it kept into a
// transaction it does not end, and its rollback would drop what it kept while
// the writes it was about stayed, for whoever does end the transaction to
// commit without it. Kept work is what makes the rest of a transaction
// acceptable -- a trail, say -- and a write committed without it is one the
// record does not have.
//
// A transaction is begun with BeginTx, and a generated client is put on it
// with WithDriver; then a stack inside it joins that one.
var ErrForeignTx = errors.New("dialect: the driver is inside a transaction BeginTx did not begin, and a transaction begun inside it would keep its work for a commit that is not the transaction's")

// txDriver is a transaction wearing a driver's clothes.
type txDriver struct {
	// drv is the driver the transaction was begun from. It is kept for the
	// dialect, which is a property of the connection rather than of the
	// transaction, and which callers ask for through however many wrappers.
	drv Driver
	// tx is the transaction. Only the caller of BeginTx holds it as one; from
	// here it is only ever read and written through.
	tx Tx
	// keep is what the transaction holds for the rest of itself, shared by
	// every driver BeginTx answered with inside it.
	keep *Keep
	// owns is this driver being the one BeginTx began the transaction on,
	// which every statement in the transaction passes through: the one that
	// asks the keep about each before it is sent.
	owns bool
}

var _ InTxDriver = (*txDriver)(nil)

func (*txDriver) InTx() bool { return true }

// Tx answers with a transaction that reads and writes through this one but
// cannot end it. See BeginTx.
func (d *txDriver) Tx(context.Context) (Tx, error) { return NopTx(d), nil }

// Dialect is the dialect of the driver the transaction was begun from.
func (d *txDriver) Dialect() string { return d.drv.Dialect() }

// Close is a nop: the connection is not this driver's to close, and the
// transaction is not this driver's to end.
func (*txDriver) Close() error { return nil }

func (d *txDriver) Exec(ctx context.Context, query string, args, v any) error {
	if d.owns {
		if err := d.keep.statement(ctx, query); err != nil {
			return err
		}
	}

	return d.tx.Exec(ctx, query, args, v)
}

func (d *txDriver) Query(ctx context.Context, query string, args, v any) error {
	if d.owns {
		if err := d.keep.statement(ctx, query); err != nil {
			return err
		}
	}

	return d.tx.Query(ctx, query, args, v)
}

// DebugDriver is a driver that logs all driver operations.
type DebugDriver struct {
	Driver                               // underlying driver.
	log    func(context.Context, ...any) // log function. defaults to log.Println.
}

// Debug gets a driver and an optional logging function, and returns
// a new debugged-driver that prints all outgoing operations.
func Debug(d Driver, logger ...func(...any)) Driver {
	logf := log.Println
	if len(logger) == 1 {
		logf = logger[0]
	}
	drv := &DebugDriver{d, func(_ context.Context, v ...any) { logf(v...) }}
	return drv
}

// DebugWithContext gets a driver and a logging function, and returns
// a new debugged-driver that prints all outgoing operations with context.
func DebugWithContext(d Driver, logger func(context.Context, ...any)) Driver {
	drv := &DebugDriver{d, logger}
	return drv
}

// Exec logs its params and calls the underlying driver Exec method.
func (d *DebugDriver) Exec(ctx context.Context, query string, args, v any) error {
	d.log(ctx, fmt.Sprintf("driver.Exec: query=%v args=%v", query, args))
	return d.Driver.Exec(ctx, query, args, v)
}

// ExecContext logs its params and calls the underlying driver ExecContext method if it is supported.
func (d *DebugDriver) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	drv, ok := d.Driver.(interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	})
	if !ok {
		return nil, fmt.Errorf("Driver.ExecContext is not supported")
	}
	d.log(ctx, fmt.Sprintf("driver.ExecContext: query=%v args=%v", query, args))
	return drv.ExecContext(ctx, query, args...)
}

// Query logs its params and calls the underlying driver Query method.
func (d *DebugDriver) Query(ctx context.Context, query string, args, v any) error {
	d.log(ctx, fmt.Sprintf("driver.Query: query=%v args=%v", query, args))
	return d.Driver.Query(ctx, query, args, v)
}

// QueryContext logs its params and calls the underlying driver QueryContext method if it is supported.
func (d *DebugDriver) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	drv, ok := d.Driver.(interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	})
	if !ok {
		return nil, fmt.Errorf("Driver.QueryContext is not supported")
	}
	d.log(ctx, fmt.Sprintf("driver.QueryContext: query=%v args=%v", query, args))
	return drv.QueryContext(ctx, query, args...)
}

// Tx adds an log-id for the transaction and calls the underlying driver Tx command.
func (d *DebugDriver) Tx(ctx context.Context) (Tx, error) {
	tx, err := d.Driver.Tx(ctx)
	if err != nil {
		return nil, err
	}
	id := uuid.New().String()
	d.log(ctx, fmt.Sprintf("driver.Tx(%s): started", id))
	return &DebugTx{tx, id, d.log, ctx}, nil
}

// BeginTx adds an log-id for the transaction and calls the underlying driver BeginTx command if it is supported.
func (d *DebugDriver) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	drv, ok := d.Driver.(interface {
		BeginTx(context.Context, *sql.TxOptions) (Tx, error)
	})
	if !ok {
		return nil, fmt.Errorf("Driver.BeginTx is not supported")
	}
	tx, err := drv.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	id := uuid.New().String()
	d.log(ctx, fmt.Sprintf("driver.BeginTx(%s): started", id))
	return &DebugTx{tx, id, d.log, ctx}, nil
}

// DebugTx is a transaction implementation that logs all transaction operations.
type DebugTx struct {
	Tx                                // underlying transaction.
	id  string                        // transaction logging id.
	log func(context.Context, ...any) // log function. defaults to fmt.Println.
	ctx context.Context               // underlying transaction context.
}

// Exec logs its params and calls the underlying transaction Exec method.
func (d *DebugTx) Exec(ctx context.Context, query string, args, v any) error {
	d.log(ctx, fmt.Sprintf("Tx(%s).Exec: query=%v args=%v", d.id, query, args))
	return d.Tx.Exec(ctx, query, args, v)
}

// ExecContext logs its params and calls the underlying transaction ExecContext method if it is supported.
func (d *DebugTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	drv, ok := d.Tx.(interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	})
	if !ok {
		return nil, fmt.Errorf("Tx.ExecContext is not supported")
	}
	d.log(ctx, fmt.Sprintf("Tx(%s).ExecContext: query=%v args=%v", d.id, query, args))
	return drv.ExecContext(ctx, query, args...)
}

// Query logs its params and calls the underlying transaction Query method.
func (d *DebugTx) Query(ctx context.Context, query string, args, v any) error {
	d.log(ctx, fmt.Sprintf("Tx(%s).Query: query=%v args=%v", d.id, query, args))
	return d.Tx.Query(ctx, query, args, v)
}

// QueryContext logs its params and calls the underlying transaction QueryContext method if it is supported.
func (d *DebugTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	drv, ok := d.Tx.(interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	})
	if !ok {
		return nil, fmt.Errorf("Tx.QueryContext is not supported")
	}
	d.log(ctx, fmt.Sprintf("Tx(%s).QueryContext: query=%v args=%v", d.id, query, args))
	return drv.QueryContext(ctx, query, args...)
}

// Commit logs this step and calls the underlying transaction Commit method.
func (d *DebugTx) Commit() error {
	d.log(d.ctx, fmt.Sprintf("Tx(%s): committed", d.id))
	return d.Tx.Commit()
}

// Rollback logs this step and calls the underlying transaction Rollback method.
func (d *DebugTx) Rollback() error {
	d.log(d.ctx, fmt.Sprintf("Tx(%s): rollbacked", d.id))
	return d.Tx.Rollback()
}
