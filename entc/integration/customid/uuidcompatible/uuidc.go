// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package uuidc

import (
	"database/sql/driver"

	"github.com/google/uuid"
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
	return u.uuid.Scan(src)
}

func (u UuidC) Value() (driver.Value, error) {
	return u.uuid.Value()
}
