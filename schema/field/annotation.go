// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package field

import "github.com/protobuf-orm/ent/schema"

// Annotation is a builtin schema annotation for
// configuring the schema fields in codegen.
type Annotation struct {
	// The StructTag option allows overriding the struct-tag
	// of the fields in the generated entity. For example:
	//
	//	field.Annotation{
	//		StructTag: map[string]string{
	//			"id": `json:"id,omitempty" yaml:"-"`,
	//		},
	//	}
	//
	StructTag map[string]string

	// Id defines a multi-field schema identifier. Note,
	// the annotation is valid only for edge schemas.
	//
	//	func (TweetLike) Annotations() []schema.Annotation {
	//		return []schema.Annotation{
	//			field.Id("user_id", "tweet_id"),
	//		}
	//	}
	//
	Id []string
}

// Id defines a multi-field schema identifier. Note, the
// annotation is valid only for edge schemas.
//
//	func (TweetLike) Annotations() []schema.Annotation {
//		return []schema.Annotation{
//			field.Id("user_id", "tweet_id"),
//		}
//	}
func Id(first, second string, fields ...string) *Annotation {
	return &Annotation{Id: append([]string{first, second}, fields...)}
}

// Name describes the annotation name.
func (Annotation) Name() string {
	return "Fields"
}

// Merge implements the schema.Merger interface.
func (a Annotation) Merge(other schema.Annotation) schema.Annotation {
	var ant Annotation
	switch other := other.(type) {
	case Annotation:
		ant = other
	case *Annotation:
		if other != nil {
			ant = *other
		}
	default:
		return a
	}
	if a.StructTag == nil && len(ant.StructTag) > 0 {
		a.StructTag = make(map[string]string, len(ant.StructTag))
	}
	for k, v := range ant.StructTag {
		a.StructTag[k] = v
	}
	if len(ant.Id) > 0 {
		a.Id = ant.Id
	}
	return a
}

var _ interface {
	schema.Annotation
	schema.Merger
} = (*Annotation)(nil)
