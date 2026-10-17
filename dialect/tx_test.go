// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package dialect_test

import (
	"context"
	"errors"
	"testing"

	"github.com/protobuf-orm/ent/dialect"
)

// The doubles below record what was asked of them rather than talking to a
// database. What this package promises is about who may end a transaction, and
// that is a question about calls, not about rows.

type recTx struct {
	commits   int
	rollbacks int
	execs     int
	queries   int
}

func (t *recTx) Commit() error   { t.commits++; return nil }
func (t *recTx) Rollback() error { t.rollbacks++; return nil }

func (t *recTx) Exec(context.Context, string, any, any) error  { t.execs++; return nil }
func (t *recTx) Query(context.Context, string, any, any) error { t.queries++; return nil }

type recDriver struct {
	tx    recTx
	began int

	dialect string
	err     error

	execs   int
	queries int
}

func (d *recDriver) Tx(context.Context) (dialect.Tx, error) {
	if d.err != nil {
		return nil, d.err
	}
	d.began++
	return &d.tx, nil
}

func (d *recDriver) Dialect() string { return d.dialect }
func (d *recDriver) Close() error    { return nil }

func (d *recDriver) Exec(context.Context, string, any, any) error  { d.execs++; return nil }
func (d *recDriver) Query(context.Context, string, any, any) error { d.queries++; return nil }

var _ dialect.Driver = (*recDriver)(nil)
var _ dialect.Tx = (*recTx)(nil)

func TestBegin(t *testing.T) {
	ctx := context.Background()

	t.Run("the transaction is the caller's", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		_, tx, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if drv.began != 1 {
			t.Fatalf("began = %d, want 1", drv.began)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %s", err)
		}
		if drv.tx.commits != 1 {
			t.Errorf("commits = %d, want 1", drv.tx.commits)
		}
	})

	t.Run("a transaction begun inside cannot end it", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, _, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}

		// What ent does on `Client.Tx`: ask the driver for a transaction and
		// end it. The generated Apply does exactly this, and it must not reach
		// the transaction the caller is holding.
		inner, err := d.Tx(ctx)
		if err != nil {
			t.Fatalf("inner tx: %s", err)
		}
		if err := inner.Commit(); err != nil {
			t.Errorf("inner commit: %s", err)
		}
		if err := inner.Rollback(); err != nil {
			t.Errorf("inner rollback: %s", err)
		}

		if drv.tx.commits != 0 {
			t.Errorf("commits = %d, want 0: an inner call ended the outer transaction", drv.tx.commits)
		}
		if drv.tx.rollbacks != 0 {
			t.Errorf("rollbacks = %d, want 0: an inner call ended the outer transaction", drv.tx.rollbacks)
		}
		// And it was the one transaction throughout.
		if drv.began != 1 {
			t.Errorf("began = %d, want 1", drv.began)
		}
	})

	t.Run("a transaction begun inside still reads and writes through it", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, _, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		inner, err := d.Tx(ctx)
		if err != nil {
			t.Fatalf("inner tx: %s", err)
		}
		if err := inner.Exec(ctx, "", nil, nil); err != nil {
			t.Fatalf("exec: %s", err)
		}
		if err := inner.Query(ctx, "", nil, nil); err != nil {
			t.Fatalf("query: %s", err)
		}

		if drv.tx.execs != 1 || drv.tx.queries != 1 {
			t.Errorf("tx got %d execs and %d queries, want 1 and 1", drv.tx.execs, drv.tx.queries)
		}
		if drv.execs != 0 || drv.queries != 0 {
			t.Errorf("the connection was used directly: %d execs, %d queries", drv.execs, drv.queries)
		}
	})

	t.Run("nesting Begin does not end the outer one either", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		outer, _, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("outer: %s", err)
		}
		_, inner, err := dialect.BeginTx(ctx, outer)
		if err != nil {
			t.Fatalf("inner: %s", err)
		}
		if err := inner.Commit(); err != nil {
			t.Fatalf("inner commit: %s", err)
		}

		if drv.tx.commits != 0 {
			t.Errorf("commits = %d, want 0", drv.tx.commits)
		}
		if drv.began != 1 {
			t.Errorf("began = %d, want 1", drv.began)
		}
	})

	t.Run("reads and writes on the driver go through the transaction", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, _, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if err := d.Exec(ctx, "", nil, nil); err != nil {
			t.Fatalf("exec: %s", err)
		}
		if err := d.Query(ctx, "", nil, nil); err != nil {
			t.Fatalf("query: %s", err)
		}

		if drv.tx.execs != 1 || drv.tx.queries != 1 {
			t.Errorf("tx got %d execs and %d queries, want 1 and 1", drv.tx.execs, drv.tx.queries)
		}
		if drv.execs != 0 || drv.queries != 0 {
			t.Errorf("the connection was used directly: %d execs, %d queries", drv.execs, drv.queries)
		}
	})

	t.Run("the dialect is the connection's", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.Postgres}

		d, _, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if got := d.Dialect(); got != dialect.Postgres {
			t.Errorf("dialect = %q, want %q", got, dialect.Postgres)
		}
	})

	t.Run("closing is not the driver's to do", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, _, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if err := d.Close(); err != nil {
			t.Errorf("close: %s", err)
		}
	})

	t.Run("a transaction that cannot be begun is reported", func(t *testing.T) {
		want := errors.New("no")
		drv := &recDriver{dialect: dialect.SQLite, err: want}

		d, tx, err := dialect.BeginTx(ctx, drv)
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
		if d != nil || tx != nil {
			t.Errorf("got a driver or a transaction along with the error")
		}
	})
}

func TestInTx(t *testing.T) {
	ctx := context.Background()

	t.Run("a plain driver is not in one", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}
		if dialect.InTx(drv) {
			t.Error("InTx = true")
		}
	})

	t.Run("one from Begin is", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}
		d, _, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if !dialect.InTx(d) {
			t.Error("InTx = false")
		}
	})

	t.Run("through the wrappers a driver picks up", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}
		d, _, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		if !dialect.InTx(dialect.Debug(d)) {
			t.Error("InTx = false through Debug")
		}
		if dialect.InTx(dialect.Debug(drv)) {
			t.Error("InTx = true for a debugged plain driver")
		}
	})
}

func TestKeep(t *testing.T) {
	ctx := context.Background()

	t.Run("the work is done before the commit", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, tx, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		k, ok := dialect.KeepOf(d)
		if !ok {
			t.Fatal("a transaction BeginTx began keeps nothing")
		}

		before := -1
		k.BeforeCommit(func(ctx context.Context) error {
			before = drv.tx.commits
			return d.Exec(ctx, "INSERT", []any{}, nil)
		})
		if drv.tx.execs != 0 {
			t.Fatalf("execs = %d before the commit, want 0: the work was done when it was kept", drv.tx.execs)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %s", err)
		}
		if drv.tx.execs != 1 {
			t.Errorf("execs = %d, want 1: the work was not done, or not in the transaction", drv.tx.execs)
		}
		if before != 0 {
			t.Errorf("the work saw %d commits, want 0: it ran after the commit", before)
		}
		if drv.tx.commits != 1 {
			t.Errorf("commits = %d, want 1", drv.tx.commits)
		}
	})

	t.Run("and not done when the transaction is rolled back", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, tx, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		k, _ := dialect.KeepOf(d)
		k.BeforeCommit(func(ctx context.Context) error { return d.Exec(ctx, "INSERT", []any{}, nil) })

		if err := tx.Rollback(); err != nil {
			t.Fatalf("rollback: %s", err)
		}
		// And a Commit after it -- the deferred one a caller wrote first --
		// finds nothing left to do.
		_ = tx.Commit()

		if drv.tx.execs != 0 {
			t.Errorf("execs = %d, want 0: work kept for a commit ran without one", drv.tx.execs)
		}
	})

	t.Run("a refusal from it is the commit's", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, tx, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		k, _ := dialect.KeepOf(d)
		refused := errors.New("refused")
		k.BeforeCommit(func(context.Context) error { return refused })

		if err := tx.Commit(); !errors.Is(err, refused) {
			t.Fatalf("commit = %v, want the work's refusal", err)
		}
		if drv.tx.commits != 0 {
			t.Errorf("commits = %d, want 0: a commit whose work was refused went ahead", drv.tx.commits)
		}
		if drv.tx.rollbacks != 1 {
			t.Errorf("rollbacks = %d, want 1: a refused commit left its transaction open", drv.tx.rollbacks)
		}
	})

	t.Run("a transaction begun inside keeps with the one around it", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, tx, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		inner, innerTx, err := dialect.BeginTx(ctx, d)
		if err != nil {
			t.Fatalf("begin inside: %s", err)
		}

		outer, _ := dialect.KeepOf(d)
		k, ok := dialect.KeepOf(inner)
		if !ok || k != outer {
			t.Fatal("a transaction begun inside keeps apart from the one it is in")
		}

		k.BeforeCommit(func(ctx context.Context) error { return inner.Exec(ctx, "INSERT", []any{}, nil) })

		// The inner commit is the one that cannot end the transaction, and so
		// it is not the one that does the work either.
		if err := innerTx.Commit(); err != nil {
			t.Fatalf("inner commit: %s", err)
		}
		if drv.tx.execs != 0 {
			t.Fatalf("execs = %d after the inner commit, want 0", drv.tx.execs)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %s", err)
		}
		if drv.tx.execs != 1 {
			t.Errorf("execs = %d, want 1: the work kept inside was not done", drv.tx.execs)
		}
	})

	t.Run("a transaction begun afresh keeps its own", func(t *testing.T) {
		one, _, err := dialect.BeginTx(ctx, &recDriver{dialect: dialect.SQLite})
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		two, _, err := dialect.BeginTx(ctx, &recDriver{dialect: dialect.SQLite})
		if err != nil {
			t.Fatalf("begin: %s", err)
		}

		a, _ := dialect.KeepOf(one)
		b, _ := dialect.KeepOf(two)
		if a == b {
			t.Error("two transactions keep one thing")
		}
	})

	t.Run("work that adds work has it done too, in order", func(t *testing.T) {
		drv := &recDriver{dialect: dialect.SQLite}

		d, tx, err := dialect.BeginTx(ctx, drv)
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		k, _ := dialect.KeepOf(d)

		done := []string{}
		k.BeforeCommit(func(context.Context) error {
			done = append(done, "first")
			k.BeforeCommit(func(context.Context) error {
				done = append(done, "added by the first")
				return nil
			})
			return nil
		})
		k.BeforeCommit(func(context.Context) error {
			done = append(done, "second")
			return nil
		})

		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %s", err)
		}
		want := []string{"first", "second", "added by the first"}
		if len(done) != len(want) || done[0] != want[0] || done[1] != want[1] || done[2] != want[2] {
			t.Errorf("done = %q, want %q", done, want)
		}
	})

	t.Run("a value is made once", func(t *testing.T) {
		d, _, err := dialect.BeginTx(ctx, &recDriver{dialect: dialect.SQLite})
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		k, _ := dialect.KeepOf(d)

		type key struct{}
		made := 0
		mk := func() any {
			made++
			// Saying what to do before the commit is what making a value is
			// usually for, and it must not wait on the value being made.
			k.BeforeCommit(func(context.Context) error { return nil })
			return &made
		}
		a := k.Value(key{}, mk)
		b := k.Value(key{}, mk)
		if made != 1 || a != b {
			t.Errorf("made = %d, and the two answers are the same: %v", made, a == b)
		}
	})

	t.Run("it is found through a DebugDriver", func(t *testing.T) {
		d, _, err := dialect.BeginTx(ctx, &recDriver{dialect: dialect.SQLite})
		if err != nil {
			t.Fatalf("begin: %s", err)
		}
		k, _ := dialect.KeepOf(d)
		if v, ok := dialect.KeepOf(dialect.Debug(d)); !ok || v != k {
			t.Error("a driver under a DebugDriver keeps nothing")
		}
	})

	t.Run("nothing keeps outside a transaction", func(t *testing.T) {
		if _, ok := dialect.KeepOf(&recDriver{dialect: dialect.SQLite}); ok {
			t.Error("a driver in no transaction keeps something")
		}
		if _, ok := dialect.KeepOf(dialect.Debug(&recDriver{dialect: dialect.SQLite})); ok {
			t.Error("a driver in no transaction, under a DebugDriver, keeps something")
		}
	})
}
