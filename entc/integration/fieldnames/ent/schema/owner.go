// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/edge"
	"github.com/protobuf-orm/ent/schema/field"
)

// Owner has the two things an update builder asks the mutation about by name:
// whether a field with an update default was cleared, which for `field` is
// FieldCleared, and whether a required unique edge was, which for `edge` is
// EdgeCleared.
type Owner struct {
	ent.Schema
}

// Fields of the Owner.
func (Owner) Fields() []ent.Field {
	return []ent.Field{
		field.Int("field").Optional().Nillable().UpdateDefault(func() int { return 1 }),
	}
}

// Edges of the Owner.
func (Owner) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("edge", Item.Type).Unique().Required(),
	}
}
