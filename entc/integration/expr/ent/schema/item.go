// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/field"
)

// Item has a field of each kind the mutation either assigns an expression or
// refuses to: one that is free to take one, one that can also be cleared, a
// JSON column, one an update fills with a default, and one validated and one
// immutable, which are refused.
type Item struct {
	ent.Schema
}

// Fields of the Item.
func (Item) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(10),
		field.String("code").Immutable(),
		field.String("label").Default(""),
		field.String("note").Optional().Nillable(),
		field.Strings("tags").Optional(),
		field.Int("revision").
			Default(0).
			UpdateDefault(func() int { return -1 }),
	}
}
