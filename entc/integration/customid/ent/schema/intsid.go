// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/entc/integration/customid/sid"
	"github.com/protobuf-orm/ent/schema/edge"
	"github.com/protobuf-orm/ent/schema/field"
)

// IntSId holds the schema definition for the IntSId entity.
type IntSId struct {
	ent.Schema
}

// Fields of the IntSid.
func (IntSId) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			GoType(sid.New()).
			Unique().
			Immutable(),
	}
}

// Edges of the IntSid.
func (IntSId) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("parent", IntSId.Type).
			Unique(),
		edge.From("children", IntSId.Type).
			Ref("parent"),
	}
}
