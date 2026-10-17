// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package dialect

import (
	"context"
	"sync"
)

// Keep is what a transaction holds for the rest of itself: values by key, and
// work to do before it commits.
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
// Only a transaction begun with [BeginTx] keeps anything. One a generated
// client began itself -- `Client.Tx`, a single write that needs a transaction
// of its own -- keeps nothing, and [KeepOf] says so: what such a write has to
// say, it says at once.
type Keep struct {
	mu     sync.Mutex
	values map[any]any
	before []func(context.Context) error
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

// finish does what is to be done before the commit, including whatever that
// adds on the way, and empties the keep.
func (k *Keep) finish(ctx context.Context) error {
	for i := 0; ; i++ {
		k.mu.Lock()
		if i == len(k.before) {
			k.before, k.values = nil, nil
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

	k.before, k.values = nil, nil
}

// keptTx is the transaction [BeginTx] answers its caller with, when it began
// one: the driver's, doing what its [Keep] holds before it commits.
type keptTx struct {
	Tx

	// ctx is the one the transaction was begun with, which is what the work
	// before the commit runs under: database/sql gives Commit none of its own.
	ctx  context.Context
	keep *Keep
}

// Commit does the kept work and then commits, and rolls back instead when the
// work fails: a transaction whose commit was refused is over either way, and
// leaving it open would hold its connection until somebody noticed.
func (t *keptTx) Commit() error {
	if err := t.keep.finish(t.ctx); err != nil {
		_ = t.Tx.Rollback()
		t.keep.discard()

		return err
	}

	return t.Tx.Commit()
}

// Rollback rolls back, and drops the kept work unrun.
func (t *keptTx) Rollback() error {
	t.keep.discard()

	return t.Tx.Rollback()
}
