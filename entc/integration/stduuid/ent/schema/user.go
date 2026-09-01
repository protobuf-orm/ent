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
// That type marshals to and from text, but implements neither
// driver.Valuer nor sql.Scanner, so every field below reaches the
// database through an external ValueScanner.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Uuid("id", uuid.UUID{}).
			Default(uuid.New).
			ValueScanner(field.TextValueScannerOf[uuid.UUID]()),
		field.String("name"),
		field.Uuid("ref", uuid.UUID{}).
			Optional().
			ValueScanner(field.TextValueScannerOf[uuid.UUID]()),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("spouse", User.Type).
			Unique(),
	}
}
