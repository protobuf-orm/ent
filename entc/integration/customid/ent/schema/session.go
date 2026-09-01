// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"database/sql/driver"
	"fmt"

	"uuid"

	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/edge"
	"github.com/protobuf-orm/ent/schema/field"
)

// Session holds the schema definition for the Session entity.
type Session struct {
	ent.Schema
}

// Fields of the Session.
func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.Bytes("id").
			MaxLen(64).
			GoType(Id{}).
			DefaultFunc(NewId),
	}
}

// Edges of the Session.
func (Session) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("device", Device.Type).
			Ref("sessions").
			Unique(),
	}
}

// Device holds the schema definition for the Device entity.
type Device struct {
	ent.Schema
}

// Fields of the Device.
func (Device) Fields() []ent.Field {
	return []ent.Field{
		field.Bytes("id").
			MaxLen(64).
			GoType(Id{}).
			DefaultFunc(NewId),
	}
}

// Edges of the Device.
func (Device) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("active_session", Session.Type).
			Unique(),
		edge.To("sessions", Session.Type),
	}
}

type Id [64]byte

func NewId() Id {
	var id [64]byte
	copy(id[:], uuid.New().String()+uuid.New().String()+uuid.New().String()+uuid.New().String())
	return id
}

func (i Id) String() string {
	return string(i[:])
}

func (i *Id) Scan(v any) error {
	switch v := v.(type) {
	case []byte:
		copy(i[:], v)
	case string:
		copy(i[:], v)
	default:
		return fmt.Errorf("unexpected type: %T", v)
	}
	return nil
}

func (i Id) Value() (driver.Value, error) {
	return string(i[:]), nil
}
