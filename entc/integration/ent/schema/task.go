// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"time"

	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/field"
	"github.com/protobuf-orm/ent/schema/index"

	"github.com/protobuf-orm/ent/entc/integration/ent/schema/task"
)

// Task holds the schema definition for the Task entity.
type Task struct {
	ent.Schema
}

// Fields of the Task.
func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.Int("priority").
			GoType(task.Priority(0)).
			Default(int(task.PriorityMid)),
		field.JSON("priorities", map[string]task.Priority{}).
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Nillable(),
		field.String("name").
			Optional().
			Deprecated(),
		field.String("owner").
			Optional(),
		field.Int("order").
			Optional(),
		field.Int("order_option").
			Optional(),
		field.String("op").
			MaxLen(45).
			Default(""),
	}
}

// Indexes of the Task.
func (Task) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name", "owner").
			Unique(),
	}
}
