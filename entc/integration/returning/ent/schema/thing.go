// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"encoding/json"

	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/dialect/entsql"
	"github.com/protobuf-orm/ent/schema/field"
)

// Thing is a row whose stored form can differ from the values it was written
// with, in each of the ways a database makes it differ: a time held at its
// column's precision, a default only the column knows, and a document kept in
// a form of the database's own.
type Thing struct {
	ent.Schema
}

// Fields of the Thing.
func (Thing) Fields() []ent.Field {
	return []ent.Field{
		// Unique, so that an INSERT can be made to fail.
		field.String("name").Unique(),
		field.Time("at"),
		// The column has a default and the builder does not, so a row
		// written without it has one only the database could say.
		field.String("motto").
			Optional().
			Annotations(entsql.Default("cave")),
		field.Json("doc", json.RawMessage{}).
			Optional(),
	}
}
