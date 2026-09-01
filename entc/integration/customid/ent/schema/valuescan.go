// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"database/sql"
	"database/sql/driver"

	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/field"
)

// ValueScan holds the schema definition for the ValueScan entity.
type ValueScan struct {
	ent.Schema
}

// ValueScanId is a custom Id type that relies on an external ValueScanner.
type ValueScanId struct {
	V int
}

// Fields of the ValueScan.
func (ValueScan) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			GoType(ValueScanId{}).
			ValueScanner(field.ValueScannerFunc[ValueScanId, *sql.NullInt64]{
				V: func(id ValueScanId) (driver.Value, error) {
					return int64(id.V), nil
				},
				S: func(id *sql.NullInt64) (ValueScanId, error) {
					if !id.Valid {
						return ValueScanId{}, nil
					}
					return ValueScanId{V: int(id.Int64)}, nil
				},
			}),
		field.String("name"),
	}
}
