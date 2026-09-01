// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/entsql"
	"github.com/protobuf-orm/ent/schema/field"
)

// Zoo holds the schema definition for the Zoo entity.
type Zoo struct {
	ent.Schema
}

// Fields of the Zoo.
func (Zoo) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Annotations(
				entsql.DefaultExprs(map[string]string{
					dialect.MySql:    "floor(rand() * ~(1<<31))",
					dialect.SQLite:   "abs(random())",
					dialect.Postgres: "floor(random() * ~(1<<31))",
				}),
			),
	}
}
