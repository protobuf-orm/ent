// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/field"

	"uuid"
)

// Item holds the schema definition for the Item entity.
type Item struct {
	ent.Schema
}

// Fields of the Item.
func (Item) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			DefaultFunc(func() string { return uuid.New().String() }).
			MaxLen(64),
		field.String("text").
			MaxLen(128).
			Unique().
			Optional(),
	}
}
