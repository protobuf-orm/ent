// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"uuid"

	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/dialect/entsql"
	"github.com/protobuf-orm/ent/schema/field"
)

// Item is a Thing whose Id the caller makes up rather than the database, which
// is what decides how a batch matches the rows it reads back to the rows it was
// given: by the Id, where a Thing's are matched by their place.
type Item struct {
	ent.Schema
}

// Fields of the Item.
func (Item) Fields() []ent.Field {
	return []ent.Field{
		field.Uuid("id").
			Default(uuid.New),
		// Unique, so that a batch can be made to fail.
		field.String("name").Unique(),
		field.String("motto").
			Optional().
			Annotations(entsql.Default("cave")),
	}
}
