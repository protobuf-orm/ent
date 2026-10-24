// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/field"
)

// Item has a field for each name whose own mutation methods would be methods
// every mutation already has: `field` is AddField, AddedField, ClearField,
// FieldCleared, ResetField, SetField and OldField of ent.Mutation; `fields`
// is AddedFields; `edge` ClearEdge, EdgeCleared and ResetEdge; `ids` AddedIds,
// and Ids of the ent package's mutation; and `where`, `op` and `type` are
// methods themselves.
//
// Each is numeric and optional, which gives it every method a field has: Add
// and Added for a number, Clear and Cleared for a column that may be NULL. And
// nillable, so that a row read back says whether it was cleared.
type Item struct {
	ent.Schema
}

// Fields of the Item.
func (Item) Fields() []ent.Field {
	return []ent.Field{
		field.Int("field").Optional().Nillable(),
		field.Int("fields").Optional().Nillable(),
		field.Int("edge").Optional().Nillable(),
		field.Int("ids").Optional().Nillable(),
		field.Int("where").Optional().Nillable(),
		field.Int("op").Optional().Nillable(),
		field.Int("type").Optional().Nillable(),
	}
}
