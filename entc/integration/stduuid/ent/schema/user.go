// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"uuid"

	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/edge"
	"github.com/protobuf-orm/ent/schema/field"
)

// User holds a schema keyed by the uuid.UUID of the standard library.
//
// Nothing below says how a UUID reaches a column, because database/sql
// reads and writes that type itself, the way it does a time.Time. A type
// of one's own does not get the same treatment and has to say.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Uuid("id").
			Default(uuid.New),
		field.String("name"),
		field.Uuid("ref").
			Optional(),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("spouse", User.Type).
			Unique(),
	}
}
