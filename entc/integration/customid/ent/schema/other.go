// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/entc/integration/customid/sid"
	"github.com/protobuf-orm/ent/schema/field"
)

// Other holds the schema definition for the Other entity.
type Other struct {
	ent.Schema
}

// Fields of the Other.
func (Other) Fields() []ent.Field {
	return []ent.Field{
		field.Other("id", sid.Id("")).
			SchemaType(map[string]string{
				dialect.MySql:    "bigint",
				dialect.Postgres: "bigint",
				dialect.SQLite:   "integer",
			}).
			Default(sid.New).
			Immutable(),
	}
}
