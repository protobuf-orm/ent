// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package ent

import (
	"context"
	"errors"

	"github.com/protobuf-orm/ent/dialect"
)

// Txn is a transaction of a generated client: it ends, and while it is open it
// hands out a client that runs through it.
type Txn[C any] interface {
	Commit() error
	Rollback() error
	Client() C
}

// Db is a generated client. It says whether it is already running inside a
// transaction, starts one if it is not, and says what it runs through.
//
// The type parameter is the client's own type rather than an interface, so that
// [JoinTx] can answer with the client it was given when it starts nothing.
type Db[C any, T Txn[C]] interface {
	InTx() bool
	Tx(context.Context) (T, error)

	// Driver is what the client runs through, which is where a transaction
	// it joined keeps what it keeps ([dialect.KeepOf]).
	Driver() dialect.Driver
}

// Joined is a transaction to work on -- or the absence of one, said the same
// way, so that a caller writes one shape either way.
type Joined[C any] struct {
	// Db is what to do the work with: the transaction's client when one was
	// started, and the client [JoinTx] was given when it was not. It is never
	// the zero value of C for a Joined that JoinTx answered with.
	Db C

	// tx is nil unless this started one, which is the whole of the difference
	// between the two cases.
	tx interface {
		Commit() error
		Rollback() error
	}

	// held is the transaction this joined, when [dialect.BeginTx] began it,
	// and how far the work went in it: what [Joined.Close] reads to tell that
	// transaction a write failed partway inside it. Nil when this began its
	// own, which Close takes back, and when it joined one that keeps nothing,
	// whose owner is the only one who can answer for it.
	held *held
}

// held is what a [Joined] that joined a kept transaction knows of its work,
// shared by its copies: a caller holds the Joined by value.
type held struct {
	keep    *dialect.Keep
	writing bool
	done    bool
}

// ErrPartial is the commit of a transaction begun with [dialect.BeginTx]
// refused, because a write inside it -- one that joined it rather than began
// its own -- failed after it had begun writing.
//
// That write cannot take back what it wrote: the transaction is not its to
// end, and a statement that holds stays until the end does. Committing anyway
// would make part of a write that answered with an error hold, so the commit
// is refused instead and the transaction rolled back, which is the answer
// PostgreSQL gives a failed statement on its own. The write's own error is the
// one its caller was handed; this is what the commit says when a caller went
// on regardless.
var ErrPartial = errors.New("ent: a write inside this transaction failed partway, and the transaction cannot be committed with part of it")

// Writing says the work is about to write: from here, failing may leave part
// of it in the transaction.
//
// Said by the work, before the first statement that writes. A failure before
// then has written nothing, and taking it back is nothing to ask of anyone.
// When this began its own transaction there is nothing to do either, since
// [Joined.Close] takes back whatever was written. When it joined one that
// [dialect.BeginTx] began, Close tells that one it cannot be committed
// ([ErrPartial]) unless [Joined.Commit] said the work holds.
func (j Joined[C]) Writing() {
	if j.held != nil {
		j.held.writing = true
	}
}

// Commit makes the work hold, and does nothing at all when this started no
// transaction of its own.
//
// A transaction is ended by whoever began it. Joining one somebody else began
// and committing it here would end it under them, half done.
func (j Joined[C]) Commit() error {
	if j.tx == nil {
		if j.held != nil {
			j.held.done = true
		}

		return nil
	}

	return j.tx.Commit()
}

// Close is what a caller defers. It takes the work back unless [Joined.Commit]
// already made it hold.
//
// When this started no transaction there is nothing of its own to take back.
// Work that joined one [dialect.BeginTx] began, and had begun writing
// ([Joined.Writing]) when it failed, leaves what it wrote there -- so Close
// tells that transaction it cannot be committed, which is the only way left to
// take the write back ([dialect.Keep.Refuse], [ErrPartial]).
//
// What a transaction that was already committed answers is not looked at: it is
// "you have ended this", which is the case this is written for.
func (j Joined[C]) Close() {
	if j.tx != nil {
		_ = j.tx.Rollback()

		return
	}
	if v := j.held; v != nil && v.writing && !v.done {
		v.keep.Refuse(ErrPartial)
	}
}

// JoinTx answers with a transaction to do the work on.
//
//	tx, err := ent.JoinTx[*ent.Client, *ent.Tx](ctx, db, true)
//	if err != nil {
//		return err
//	}
//	defer tx.Close()
//
//	// ... what is read and checked first, with tx.Db ...
//
//	tx.Writing()
//	// ... the writes ...
//
//	return tx.Commit()
//
// It starts nothing, and answers with the client it was given, in two cases.
// One is a client already inside a transaction: whoever began that one ends it,
// and a write made here belongs to it. The other is want being false, which is
// for a caller whose need for a transaction depends on something -- a write
// alone is one statement and needs none, and the same write together with a
// record of it is two and does. Either way the caller commits and closes the
// same way, and neither ends anything.
//
// What it can do, inside a transaction [dialect.BeginTx] began, is refuse that
// one's commit, when the work failed after it began writing; see
// [Joined.Close].
func JoinTx[C Db[C, T], T Txn[C]](ctx context.Context, db C, want bool) (Joined[C], error) {
	if db.InTx() {
		j := Joined[C]{Db: db}
		if k, ok := dialect.KeepOf(db.Driver()); ok {
			j.held = &held{keep: k}
		}

		return j, nil
	}
	if !want {
		return Joined[C]{Db: db}, nil
	}

	tx, err := db.Tx(ctx)
	if err != nil {
		return Joined[C]{}, err
	}

	return Joined[C]{Db: tx.Client(), tx: tx}, nil
}
