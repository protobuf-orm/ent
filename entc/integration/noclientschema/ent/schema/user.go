// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/field"
)

// User is here to be generated, not to be interesting.
//
// What this package tests is the shape of the client around it: with
// `sql/no-client-schema` on, Client has no Schema field and the generated
// package does not import its own migrate, so nothing pulls Atlas in behind it.
// One entity with one field is enough to make a client.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}
