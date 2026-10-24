// Command linked does with a schema what a generated client does at init, for
// TestMakingFieldsLetsTheLinkerPruneMethods to read what the linker keeps of
// it.
package main

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"uuid"

	"github.com/protobuf-orm/ent"
	"github.com/protobuf-orm/ent/schema/field"
)

// Schema has a field of each builder that records a Go type, with a default and
// a value scanner, which are what runtime.go reads off a descriptor.
type Schema struct {
	ent.Schema
}

func (Schema) Fields() []ent.Field {
	return []ent.Field{
		field.Uuid("id").Default(uuid.New),
		field.Json("doc", &Doc{}),
		field.Other("money", Money(0)).SchemaType(map[string]string{"sqlite3": "integer"}),
		field.String("name").GoType(&Name{}).ValueScanner(field.TextValueScanner[*Name]{}),
		field.Enum("kind").GoType(Kind("")),
		field.Int("count").GoType(Count(0)),
		field.Uint32("domain").Optional().Default(0),
	}
}

type Doc struct {
	Title string
}

type Money int64

func (m Money) Value() (driver.Value, error) { return int64(m), nil }

func (m *Money) Scan(src any) error {
	v, ok := src.(int64)
	if !ok {
		return fmt.Errorf("money: %T", src)
	}
	*m = Money(v)
	return nil
}

type Name struct {
	s string
}

func (n *Name) MarshalText() ([]byte, error) { return []byte(n.s), nil }

func (n *Name) UnmarshalText(b []byte) error {
	n.s = string(b)
	return nil
}

type Kind string

func (Kind) Values() []string { return []string{"a", "b"} }

type Count int

func main() {
	// The descriptor of each field, and what it says.
	for _, f := range (Schema{}).Fields() {
		d := f.Descriptor()
		fmt.Println(d.Name, d.Info, d.Default != nil, d.ValueScanner != nil, d.Err)
	}

	// And what nearly every program does somewhere, encoding/json among them:
	// ask through reflect.Type whether a type implements an interface. The
	// linker then keeps a method with that name and type on any type converted
	// to an interface.
	var t reflect.Type = reflect.TypeFor[Schema]()
	fmt.Println(t.Implements(reflect.TypeFor[fmt.Stringer]()))
}
