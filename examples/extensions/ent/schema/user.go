// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Bytes("location").
			// Ideally, we would use a custom GoType
			// to represent the "geometry" type.
			SchemaType(map[string]string{
				dialect.Postgres: "GEOMETRY(Point, 4326)",
			}),
	}
}
