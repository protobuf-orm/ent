// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/google/uuid"
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/field"
	"github.com/protobuf-orm/ent/schema/index"
	"github.com/protobuf-orm/ent/schema/mixin"
)

// BaseMixin holds the schema definition for the BaseMixin entity.
type BaseMixin struct {
	mixin.Schema
}

// Fields of the Mixin.
func (BaseMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Uuid("id", uuid.UUID{}).Default(uuid.New),
		field.String("some_field"),
	}
}

// MixinId holds the schema definition for the MixinId entity.
type MixinId struct {
	ent.Schema
}

// Fields of the MixinId.
func (MixinId) Fields() []ent.Field {
	return []ent.Field{
		field.String("mixin_field"),
	}
}

// Indexes of the MixinId
func (MixinId) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("id"),
		index.Fields("id", "some_field"),
		index.Fields("id", "mixin_field"),
		index.Fields("id", "mixin_field", "some_field"),
	}
}

// Mixin of MixinId
func (MixinId) Mixin() []ent.Mixin {
	return []ent.Mixin{
		BaseMixin{},
	}
}
