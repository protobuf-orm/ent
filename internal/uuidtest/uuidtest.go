// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package uuidtest provides a database-compatible UUID type for tests.
//
// The standard library uuid package marshals to and from text, but does not
// implement driver.Valuer or sql.Scanner. Both are required by ent's UUID
// fields and by the SQL scanners, so this type adds them.
//
// It is defined over uuid.UUID rather than wrapping it in a struct, because
// the SQL scanners treat a struct as a set of columns to map a row onto.
package uuidtest

import (
	"database/sql/driver"
	"fmt"
	"uuid"
)

// UUID is a uuid.UUID that can be written to and read from a database.
type UUID uuid.UUID

// New returns a randomly generated UUID.
func New() UUID { return UUID(uuid.New()) }

// Nil is the zero UUID.
var Nil UUID

// String returns the string form of the UUID.
func (u UUID) String() string { return uuid.UUID(u).String() }

// Value implements the driver.Valuer interface.
func (u UUID) Value() (driver.Value, error) { return u.String(), nil }

// Scan implements the sql.Scanner interface.
func (u *UUID) Scan(src any) error {
	var text []byte
	switch src := src.(type) {
	case nil:
		*u = Nil
		return nil
	case string:
		text = []byte(src)
	case []byte:
		text = src
	default:
		return fmt.Errorf("uuidtest: cannot scan %T into UUID", src)
	}
	return (*uuid.UUID)(u).UnmarshalText(text)
}
