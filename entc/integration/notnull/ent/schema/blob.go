// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/field"
)

// Blob holds a schema whose columns are bytes, required and not.
//
// A required bytes column cannot hold a NULL, so it cannot hold a nil
// either -- but the drivers disagree about what a zero-length blob reads
// back as, and one of them says nil. What is asserted here is that the
// schema's answer wins over the driver's.
type Blob struct {
	ent.Schema
}

// Fields of the Blob.
func (Blob) Fields() []ent.Field {
	return []ent.Field{
		field.Bytes("data"),
		field.Bytes("note").
			Optional(),
	}
}
