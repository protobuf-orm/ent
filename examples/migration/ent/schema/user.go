// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/dialect/entsql"
	"github.com/protobuf-orm/ent/schema"
	"github.com/protobuf-orm/ent/schema/edge"
	"github.com/protobuf-orm/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Float("age"),
		field.String("first_name"),
		field.String("last_name"),
		field.Strings("tags").
			Optional(),
	}
}

// Annotations of the User.
func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		// In case schema.ModeInspect is used without a dev-database, unnamed check constraints
		// should be normalized (i.e. identical to their definition in the database). In this
		// case, it is entsql.Check("(`age` > 0)"). See: https://atlasgo.io/concepts/dev-database.
		entsql.Check("age > 0"),

		// Named check constraints are compared by their name.
		// Thus, the definition does not need to be normalized.
		entsql.Checks(map[string]string{
			"first_last_not_empty": "(`first_name` <> '' AND `last_name` <> '')",
		}),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("cards", Card.Type),
	}
}
