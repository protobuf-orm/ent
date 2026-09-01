// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"database/sql/driver"
	"fmt"

	"github.com/google/uuid"
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/schema/edge"
	"github.com/protobuf-orm/ent/schema/field"

	"ariga.io/atlas/sql/postgres"
)

// Doc holds the schema definition for the Doc entity.
type Doc struct {
	ent.Schema
}

// Fields of the Doc.
func (Doc) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			GoType(DocId("")).
			MaxLen(36).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(func() DocId {
				return DocId(uuid.NewString())
			}).
			SchemaType(map[string]string{
				dialect.Postgres: postgres.TypeUUID,
			}),
		field.String("text").
			Optional(),
	}
}

// Edges of the Doc.
func (Doc) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", Doc.Type).
			From("parent").
			Unique(),
		edge.To("related", Doc.Type),
	}
}

type DocId string

// Scan implements the Scanner interface.
func (s *DocId) Scan(value any) (err error) {
	switch v := value.(type) {
	case nil:
	case []byte:
		*s = DocId(v)
	case string:
		*s = DocId(v)
	default:
		err = fmt.Errorf("unexpected type %T", v)
	}
	return
}

// Value implements the driver Valuer interface.
func (s DocId) Value() (driver.Value, error) {
	return string(s), nil
}
