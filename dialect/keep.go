// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package dialect

import (
	"context"
	"sync"
)

// Keep is what a transaction holds for the rest of itself: values by key, work
// to do before it commits, and work to do before a statement it runs.
//
// It is for a write that has more to say than is worth saying at once. A
// record of every write is a row each, so a transaction that writes a thousand
// rows writes a thousand records as well -- a round trip each, where a
// statement could carry them all. Kept here, they are written together before
// the transaction commits.
//
// The work is part of the commit and nothing else. Whoever ends the
// transaction cannot forget it, since ending it is what does it, and a
// transaction rolled back takes it along unrun. Nor can it reach another
// transaction: one begun inside another is that one ([BeginTx]), and so is what
// it keeps, while one begun afresh keeps its own.
//
// And what is kept for the commit can be needed sooner. A row kept to be
// written is a row a statement in the same transaction does not find --
// counted, updated, erased -- until it is written, and then the statement
// answers about a table that is missing some of what the transaction did.
// [Keep.BeforeStatement] is for that: the kept rows are written first, ahead
// of any statement that says it is about them.
//
// Only a transaction begun with [BeginTx] keeps anything. One a generated
// client began itself -- `Client.Tx`, a single write that needs a transaction
// of its own -- keeps nothing, and [KeepOf] says so: what such a write has to
// say, it says at once. Nor does BeginTx begin one inside it
// ([ErrForeignTx]): what that one kept would answer to a commit that is not
// the transaction's.
type Keep struct {
	mu     sync.Mutex
	values map[any]any
	before []func(context.Context) error
	ahead  []ahead

	// busy is kept work running, whose own statements are not asked about:
	// work that writes what it kept would otherwise be asked to write it
	// again, by the statement that writes it.
	busy bool
}

// ahead is work to do before the statements match says yes to.
type ahead struct {
	match func(query string) bool
	fn    func(context.Context) error
}

// KeepOf answers with what the transaction drv runs in keeps, looking through
// the wrappers a driver picks up on the way the way [InTx] does -- and false
// when drv runs in no transaction, or in one that keeps nothing.
func KeepOf(drv Driver) (*Keep, bool) {
	for {
		switch v := drv.(type) {
		case *txDriver:
			return v.keep, v.keep != nil
		case *DebugDriver:
			drv = v.Driver
		default:
			return nil, false
		}
	}
}

// Value answers with what is kept under key, made with mk the first time it is
// asked for. mk may say what to do before the commit; it is not called with
// anything held.
func (k *Keep) Value(key any, mk func() any) any {
	k.mu.Lock()
	v, ok := k.values[key]
	k.mu.Unlock()
	if ok {
		return v
	}

	made := mk()

	k.mu.Lock()
	defer k.mu.Unlock()
	if v, ok := k.values[key]; ok {
		// Made meanwhile. A transaction is used from one goroutine at a time,
		// so this is a caller doing something it should not, and the first
		// one made is the one kept.
		return v
	}
	if k.values == nil {
		k.values = map[any]any{}
	}
	k.values[key] = made

	return made
}

// BeforeCommit adds fn to what the transaction does before it commits, after
// whatever was added before it.
//
// An error from fn is the commit's: the transaction is rolled back and nothing
// done in it holds -- which is the answer wanted when what fn writes is what
// makes the rest of the transaction acceptable.
func (k *Keep) BeforeCommit(fn func(context.Context) error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.before = append(k.before, fn)
}

// BeforeStatement adds fn to what the transaction does before a statement it
// runs, for every statement match says yes to -- and before the statement,
// so that it finds what fn wrote.
//
// match is handed the statement as it is sent. What fn sends itself is not
// asked about, and neither is what the work before the commit sends: both are
// kept work being done, and asking would have it done again from inside
// itself.
//
// An error from fn is the statement's: it is not sent, and the caller is told
// why.
func (k *Keep) BeforeStatement(match func(query string) bool, fn func(context.Context) error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.ahead = append(k.ahead, ahead{match, fn})
}

// statement does the work kept for a statement about to be sent.
func (k *Keep) statement(ctx context.Context, query string) error {
	k.mu.Lock()
	if k.busy || len(k.ahead) == 0 {
		k.mu.Unlock()

		return nil
	}
	todo := []func(context.Context) error{}
	for _, v := range k.ahead {
		if v.match(query) {
			todo = append(todo, v.fn)
		}
	}
	if len(todo) == 0 {
		k.mu.Unlock()

		return nil
	}
	k.busy = true
	k.mu.Unlock()

	defer k.idle()
	for _, fn := range todo {
		if err := fn(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (k *Keep) idle() {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.busy = false
}

// finish does what is to be done before the commit, including whatever that
// adds on the way, and empties the keep.
func (k *Keep) finish(ctx context.Context) error {
	k.mu.Lock()
	k.busy = true
	k.mu.Unlock()
	defer k.idle()

	for i := 0; ; i++ {
		k.mu.Lock()
		if i == len(k.before) {
			k.before, k.ahead, k.values = nil, nil, nil
			k.mu.Unlock()

			return nil
		}
		fn := k.before[i]
		k.mu.Unlock()

		if err := fn(ctx); err != nil {
			return err
		}
	}
}

// discard empties the keep of a transaction that will not commit.
func (k *Keep) discard() {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.before, k.ahead, k.values = nil, nil, nil
}

// keptTx is what [BeginTx] answers its caller with when it began a
// transaction: the end of it, which does what its [Keep] holds before it
// commits.
//
// It ends the transaction and does nothing else. A statement goes through the
// driver BeginTx answered with, which asks the keep about it first
// ([Keep.BeforeStatement]); one sent through this would be asked nothing, so
// this has no way to send one.
type keptTx struct {
	tx Tx

	// ctx is the one the transaction was begun with, which is what the work
	// before the commit runs under: database/sql gives Commit none of its own.
	ctx  context.Context
	keep *Keep
}

// Commit does the kept work and then commits, and rolls back instead when the
// work fails: a transaction whose commit was refused is over either way, and leaving it open would hold its connection until
// somebody noticed.
func (t *keptTx) Commit() error {
	if err := t.keep.finish(t.ctx); err != nil {
		_ = t.tx.Rollback()
		t.keep.discard()

		return err
	}

	return t.tx.Commit()
}

// Rollback rolls back, and drops the kept work unrun.
func (t *keptTx) Rollback() error {
	t.keep.discard()

	return t.tx.Rollback()
}

// inside is what [BeginTx] answers with inside a transaction it began before:
// the end of a transaction that is not this caller's to end, so ending it does
// nothing. The outer one's commit does what is kept, and its rollback drops it.
type inside struct{}

func (inside) Commit() error   { return nil }
func (inside) Rollback() error { return nil }
