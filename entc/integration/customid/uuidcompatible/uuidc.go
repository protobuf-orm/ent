// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package uuidc holds a UUID type that speaks to a database itself, rather
// than being written as text on the field's behalf.
package uuidc

import (
	"database/sql/driver"
	"fmt"
	"uuid"
)

type UuidC struct {
	uuid uuid.UUID
}

func NewUuidC() UuidC {
	return UuidC{
		uuid: uuid.New(),
	}
}

func (u *UuidC) Scan(src any) error {
	switch src := src.(type) {
	case nil:
		u.uuid = uuid.Nil()
		return nil
	case string:
		return u.uuid.UnmarshalText([]byte(src))
	case []byte:
		return u.uuid.UnmarshalText(src)
	default:
		return fmt.Errorf("uuidc: cannot scan %T into UuidC", src)
	}
}

func (u UuidC) Value() (driver.Value, error) {
	return u.uuid.String(), nil
}
