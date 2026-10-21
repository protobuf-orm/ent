// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package ent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/dialect"
)

// The doubles stand in for a generated ent client and its transaction. What
// [ent.JoinTx] promises is about who ends a transaction and who does not, which
// is a question about calls.

type recClient struct {
	inTx bool
	err  error

	// drv is what the client runs through, which is a kept transaction's
	// when the test puts it on one.
	drv dialect.Driver

	tx    recClientTx
	began int
}

func (c *recClient) InTx() bool { return c.inTx }

func (c *recClient) Driver() dialect.Driver { return c.drv }

func (c *recClient) Tx(context.Context) (*recClientTx, error) {
	c.began++
	if c.err != nil {
		return nil, c.err
	}

	c.tx.of = c
	return &c.tx, nil
}

type recClientTx struct {
	of *recClient

	commits   int
	rollbacks int
}

func (t *recClientTx) Commit() error   { t.commits++; return nil }
func (t *recClientTx) Rollback() error { t.rollbacks++; return nil }

// Client is the client to work on: a transaction's own, so that what is written
// through it is inside the transaction.
func (t *recClientTx) Client() *recClient { return &recClient{inTx: true} }

func join(t *testing.T, c *recClient, want bool) ent.Joined[*recClient] {
	t.Helper()

	v, err := ent.JoinTx[*recClient, *recClientTx](context.Background(), c, want)
	if err != nil {
		t.Fatalf("join: %s", err)
	}

	return v
}

func TestJoinStarts(t *testing.T) {
	c := &recClient{}

	v := join(t, c, true)
	if c.began != 1 {
		t.Errorf("began = %d, want 1", c.began)
	}
	if v.Db == c {
		t.Error("the work runs on the client it was given, not on the transaction's")
	}
	if !v.Db.InTx() {
		t.Error("the client it answers with is not inside the transaction")
	}

	if err := v.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	if c.tx.commits != 1 {
		t.Errorf("commits = %d, want 1", c.tx.commits)
	}

	// What a caller defers, on a transaction that already held. The answer of a
	// transaction that has ended is not looked at, which is the whole point of
	// deferring it.
	v.Close()
	if c.tx.rollbacks != 1 {
		t.Errorf("rollbacks = %d, want 1", c.tx.rollbacks)
	}
}

func TestJoinTakesBack(t *testing.T) {
	c := &recClient{}

	v := join(t, c, true)
	v.Close()

	if c.tx.rollbacks != 1 {
		t.Errorf("rollbacks = %d, want 1", c.tx.rollbacks)
	}
	if c.tx.commits != 0 {
		t.Errorf("commits = %d, want 0", c.tx.commits)
	}
}

// A client already inside a transaction is one somebody else began, and ending
// it is theirs. Committing here would end it under them, half done.
func TestJoinDoesNotEndAnothersTransaction(t *testing.T) {
	c := &recClient{inTx: true}

	v := join(t, c, true)
	if c.began != 0 {
		t.Errorf("began = %d, want 0", c.began)
	}
	if v.Db != c {
		t.Error("the work should run on the client it was given")
	}

	if err := v.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	v.Close()

	if c.tx.commits != 0 || c.tx.rollbacks != 0 {
		t.Errorf("commits = %d and rollbacks = %d, want 0 and 0", c.tx.commits, c.tx.rollbacks)
	}
}

// A caller whose need for a transaction depends on something writes the same
// shape either way, and the way that needs none costs none.
func TestJoinWantsNothing(t *testing.T) {
	c := &recClient{}

	v := join(t, c, false)
	if c.began != 0 {
		t.Errorf("began = %d, want 0", c.began)
	}
	if v.Db != c {
		t.Error("the work should run on the client it was given")
	}

	if err := v.Commit(); err != nil {
		t.Fatalf("commit: %s", err)
	}
	v.Close()

	if c.tx.commits != 0 || c.tx.rollbacks != 0 {
		t.Errorf("commits = %d and rollbacks = %d, want 0 and 0", c.tx.commits, c.tx.rollbacks)
	}
}

func TestJoinReportsWhatItCouldNotStart(t *testing.T) {
	want := errors.New("no")
	c := &recClient{err: want}

	_, err := ent.JoinTx[*recClient, *recClientTx](context.Background(), c, true)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

// conn is a connection that answers nothing, for a transaction to be begun on
// with dialect.BeginTx: what is under test is what that transaction is told.
type conn struct{ tx connTx }

type connTx struct{ commits, rollbacks int }

func (c *conn) Exec(context.Context, string, any, any) error  { return nil }
func (c *conn) Query(context.Context, string, any, any) error { return nil }
func (c *conn) Tx(context.Context) (dialect.Tx, error)        { return &c.tx, nil }
func (c *conn) Close() error                                  { return nil }
func (c *conn) Dialect() string                               { return dialect.SQLite }

func (t *connTx) Exec(context.Context, string, any, any) error  { return nil }
func (t *connTx) Query(context.Context, string, any, any) error { return nil }
func (t *connTx) Commit() error                                 { t.commits++; return nil }
func (t *connTx) Rollback() error                               { t.rollbacks++; return nil }

// Work that joined a transaction somebody else began cannot take back what it
// wrote, since ending that transaction is not its to do. When it fails after it
// began writing, the transaction is told it cannot be committed with part of
// it -- and that is all it is told.
func TestJoinRefusesWhatFailedPartway(t *testing.T) {
	ctx := context.Background()

	// A kept transaction, begun as a stack's is, and a client on it.
	kept := func(t *testing.T) (*conn, *recClient, interface{ Commit() error }) {
		t.Helper()

		c := &conn{}
		drv, tx, err := dialect.BeginTx(ctx, c)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}

		return c, &recClient{inTx: true, drv: drv}, tx
	}

	t.Run("work that failed after it began writing refuses the commit", func(t *testing.T) {
		c, db, tx := kept(t)

		v := join(t, db, true)
		v.Writing()
		v.Close()

		if err := tx.Commit(); !errors.Is(err, ent.ErrPartial) {
			t.Fatalf("commit = %v, want ErrPartial", err)
		}
		if c.tx.commits != 0 || c.tx.rollbacks != 1 {
			t.Errorf("commits = %d and rollbacks = %d, want 0 and 1: part of a failed write was committed", c.tx.commits, c.tx.rollbacks)
		}
	})

	t.Run("and work that failed before it wrote does not", func(t *testing.T) {
		c, db, tx := kept(t)

		v := join(t, db, true)
		v.Close()

		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %s", err)
		}
		if c.tx.commits != 1 {
			t.Errorf("commits = %d, want 1", c.tx.commits)
		}
	})

	t.Run("nor does work that held", func(t *testing.T) {
		c, db, tx := kept(t)

		v := join(t, db, true)
		v.Writing()
		if err := v.Commit(); err != nil {
			t.Fatalf("join commit: %s", err)
		}
		v.Close()

		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %s", err)
		}
		if c.tx.commits != 1 {
			t.Errorf("commits = %d, want 1", c.tx.commits)
		}
	})

	t.Run("and the work that wanted no transaction is told the same", func(t *testing.T) {
		_, db, tx := kept(t)

		v := join(t, db, false)
		v.Writing()
		v.Close()

		if err := tx.Commit(); !errors.Is(err, ent.ErrPartial) {
			t.Fatalf("commit = %v, want ErrPartial", err)
		}
	})

	// A transaction that keeps nothing -- a generated Client.Tx -- has nothing
	// to be told with, and its owner is the one who answers for it.
	t.Run("a transaction that keeps nothing is not told", func(t *testing.T) {
		db := &recClient{inTx: true, drv: &conn{}}

		v := join(t, db, true)
		v.Writing()
		v.Close()

		if db.tx.commits != 0 || db.tx.rollbacks != 0 {
			t.Errorf("commits = %d and rollbacks = %d, want 0 and 0", db.tx.commits, db.tx.rollbacks)
		}
	})

	// Work that began its own takes it back, as it always has, and tells
	// nobody: there is nobody else's transaction to tell.
	t.Run("work in its own transaction is taken back by it", func(t *testing.T) {
		db := &recClient{}

		v := join(t, db, true)
		v.Writing()
		v.Close()

		if db.tx.rollbacks != 1 {
			t.Errorf("rollbacks = %d, want 1", db.tx.rollbacks)
		}
	})
}
