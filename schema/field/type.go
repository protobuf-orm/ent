// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package field

import (
	"fmt"
	"reflect"
	"strings"
)

// A Type represents a field type.
type Type uint8

// List of field types.
const (
	TypeInvalid Type = iota
	TypeBool
	TypeTime
	TypeJson
	TypeUuid
	TypeBytes
	TypeEnum
	TypeString
	TypeOther
	TypeInt8
	TypeInt16
	TypeInt32
	TypeInt
	TypeInt64
	TypeUint8
	TypeUint16
	TypeUint32
	TypeUint
	TypeUint64
	TypeFloat32
	TypeFloat64
	endTypes
)

// String returns the string representation of a type.
func (t Type) String() string {
	if t < endTypes {
		return typeNames[t]
	}
	return typeNames[TypeInvalid]
}

// Numeric reports if the given type is a numeric type.
func (t Type) Numeric() bool {
	return t >= TypeInt8 && t < endTypes
}

// Float reports if the given type is a float type.
func (t Type) Float() bool {
	return t == TypeFloat32 || t == TypeFloat64
}

// Integer reports if the given type is an integral type.
func (t Type) Integer() bool {
	return t.Numeric() && !t.Float()
}

// Valid reports if the given type if known type.
func (t Type) Valid() bool {
	return t > TypeInvalid && t < endTypes
}

// ConstName returns the constant name of an info type.
// It's used by entc for printing the constant name in templates.
func (t Type) ConstName() string {
	switch {
	case !t.Valid():
		return typeNames[TypeInvalid]
	case int(t) < len(constNames) && constNames[t] != "":
		return constNames[t]
	default:
		return "Type" + strings.Title(typeNames[t])
	}
}

// TypeInfo holds the information regarding field type.
// Used by complex types like JSON and  Bytes.
type TypeInfo struct {
	Type     Type
	Ident    string
	PkgPath  string // import path.
	PkgName  string // local package name.
	Nillable bool   // slices or pointers.
	RType    *RType
}

// String returns the string representation of a type.
func (t TypeInfo) String() string {
	switch {
	case t.Ident != "":
		return t.Ident
	case t.Type < endTypes:
		return typeNames[t.Type]
	default:
		return typeNames[TypeInvalid]
	}
}

// Valid reports if the given type if known type.
func (t TypeInfo) Valid() bool {
	return t.Type.Valid()
}

// Numeric reports if the given type is a numeric type.
func (t TypeInfo) Numeric() bool {
	return t.Type.Numeric()
}

// ConstName returns the const name of the info type.
func (t TypeInfo) ConstName() string {
	return t.Type.ConstName()
}

// ValueScanner indicates if this type implements the ValueScanner interface.
func (t TypeInfo) ValueScanner() bool {
	return t.RType.implements(valueScannerType) || t.RType.DriverType()
}

// DriverType indicates if database/sql reads and writes values of this type
// itself. See RType.DriverType.
func (t TypeInfo) DriverType() bool {
	return t.RType.DriverType()
}

// Validator indicates if this type implements the Validator interface.
func (t TypeInfo) Validator() bool {
	return t.RType.implements(validatorType)
}

// Valuer indicates if this type can be handed to a driver as it is, either
// because it implements driver.Valuer or because database/sql converts it.
func (t TypeInfo) Valuer() bool {
	return t.RType.implements(valuerType) || t.RType.DriverType()
}

// Comparable reports whether values of this type are comparable.
func (t TypeInfo) Comparable() bool {
	switch t.Type {
	case TypeBool, TypeTime, TypeUuid, TypeEnum, TypeString:
		return true
	case TypeOther:
		// Always accept custom types as comparable on the database side.
		// In the future, we should consider adding an interface to let
		// custom types tell if they are comparable or not (see #1304).
		return true
	default:
		return t.Numeric()
	}
}

var stringerType = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()

// Stringer indicates if this type implements the Stringer interface.
func (t TypeInfo) Stringer() bool {
	return t.RType.implements(stringerType)
}

var (
	typeNames = [...]string{
		TypeInvalid: "invalid",
		TypeBool:    "bool",
		TypeTime:    "time.Time",
		TypeJson:    "json.RawMessage",
		TypeUuid:    "[16]byte",
		TypeBytes:   "[]byte",
		TypeEnum:    "string",
		TypeString:  "string",
		TypeOther:   "other",
		TypeInt:     "int",
		TypeInt8:    "int8",
		TypeInt16:   "int16",
		TypeInt32:   "int32",
		TypeInt64:   "int64",
		TypeUint:    "uint",
		TypeUint8:   "uint8",
		TypeUint16:  "uint16",
		TypeUint32:  "uint32",
		TypeUint64:  "uint64",
		TypeFloat32: "float32",
		TypeFloat64: "float64",
	}
	constNames = [...]string{
		TypeJson:  "TypeJson",
		TypeUuid:  "TypeUuid",
		TypeTime:  "TypeTime",
		TypeEnum:  "TypeEnum",
		TypeBytes: "TypeBytes",
		TypeOther: "TypeOther",
	}
)

// RType holds a serializable reflect.Type information of
// Go object. Used by the entc package.
type RType struct {
	Name    string // reflect.Type.Name
	Ident   string // reflect.Type.String
	Kind    reflect.Kind
	PkgPath string

	// Methods is the method set of the type -- of *T, for a T -- which is how
	// entc knows what a GoType implements. A descriptor leaves it nil, and
	// entc's loader fills it in: see LoadMethods.
	Methods map[string]struct{ In, Out []*RType }

	// Used only for in-package checks.
	rtype reflect.Type
}

// LoadMethods records the methods of the type in Methods. entc's loader calls
// it on every field before it encodes the schema for codegen, which is what
// reads them. It does nothing to an RType made from no type.
//
// Upstream records them as a descriptor is made. They are a step of their own
// here because a running program makes descriptors too: a generated client
// whose schema has a default, a validator or a value scanner calls the
// schema's Fields at init (runtime.go), and so builds every field of it.
// Listing a type's methods is reflect.Type.Method with an index the compiler
// cannot see, and in a program that can reach such a call the linker has to
// assume that any exported method of any type converted to an interface may
// be called by name -- so it keeps every one of them, used or not, in the
// whole program. In lesomnus/payday's app compiled to WebAssembly, the module
// a browser downloads, that was 24 MB of 63.
func (r *RType) LoadMethods() {
	if r == nil || r.rtype == nil {
		return
	}
	t := methodSet(r.rtype)
	n := t.NumMethod()
	r.Methods = make(map[string]struct{ In, Out []*RType }, n)
	for i := 0; i < n; i++ {
		m := t.Method(i)
		in := make([]*RType, m.Type.NumIn()-1)
		for j := range in {
			arg := m.Type.In(j + 1)
			in[j] = &RType{Name: arg.Name(), Ident: arg.String(), Kind: arg.Kind(), PkgPath: arg.PkgPath()}
		}
		out := make([]*RType, m.Type.NumOut())
		for j := range out {
			ret := m.Type.Out(j)
			out[j] = &RType{Name: ret.Name(), Ident: ret.String(), Kind: ret.Kind(), PkgPath: ret.PkgPath()}
		}
		r.Methods[m.Name] = struct{ In, Out []*RType }{in, out}
	}
}

// methodSet is the type whose methods an RType holds for t: *T for a T, so that
// a method with a pointer receiver counts, and a pointer as it is.
func methodSet(t reflect.Type) reflect.Type {
	if t.Kind() != reflect.Ptr {
		return reflect.PtrTo(t)
	}
	return t
}

// DriverType reports whether database/sql reads and writes values of this type
// itself, so that a field holding one needs nothing said about how it reaches
// a column.
//
// It is the standard library's own short list rather than a property of the
// type: a time.Time, and since Go 1.27 a uuid.UUID. Nothing can be inferred
// from the shape of a type here -- a type of one's own that is sixteen bytes
// with a MarshalText, which is exactly what a uuid.UUID is, is refused by the
// same package.
func (r *RType) DriverType() bool {
	return r != nil && r.PkgPath == "uuid" && r.Name == "UUID"
}

// TypeEqual reports if the underlying type is equal to the RType (after pointer indirections).
func (r *RType) TypeEqual(t reflect.Type) bool {
	tv := indirect(t)
	return r.Name == tv.Name() && r.Kind == t.Kind() && r.PkgPath == tv.PkgPath()
}

// RType returns the string value of the indirect reflect.Type.
func (r *RType) String() string {
	if r.rtype != nil {
		return r.rtype.String()
	}
	return r.Ident
}

// IsPtr reports if the reflect-type is a pointer type.
func (r *RType) IsPtr() bool {
	return r != nil && r.Kind == reflect.Ptr
}

// implements reports whether the RType ~implements the given interface type.
// One made from a type, as a descriptor's is, is asked through reflect, which
// lists no methods to answer; one decoded from a loaded schema, as entc has
// it, by the methods recorded in Methods. TypeInfo's ValueScanner, Validator,
// Valuer and Stringer are what entc asks.
//
// It is RType.Implements upstream, and is not exported here for that name and
// signature: they are reflect.Type's own Implements, which a program calls
// through the interface (encoding/json does), and the linker keeps a method
// of that name and type on any type converted to an interface -- which an
// RType is, behind each field's builder. The loop below is reflect.Type.Method
// with an index, so this is not for a running program to reach either: a
// check a descriptor makes of its own type asks methodSet(t).Implements.
func (r *RType) implements(typ reflect.Type) bool {
	switch {
	case r == nil:
		return false
	case r.rtype != nil:
		return methodSet(r.rtype).Implements(typ)
	}
	n := typ.NumMethod()
	for i := 0; i < n; i++ {
		m0 := typ.Method(i)
		m1, ok := r.Methods[m0.Name]
		if !ok || len(m1.In) != m0.Type.NumIn() || len(m1.Out) != m0.Type.NumOut() {
			return false
		}
		in := m0.Type.NumIn()
		for j := 0; j < in; j++ {
			if !m1.In[j].TypeEqual(m0.Type.In(j)) {
				return false
			}
		}
		out := m0.Type.NumOut()
		for j := 0; j < out; j++ {
			if !m1.Out[j].TypeEqual(m0.Type.Out(j)) {
				return false
			}
		}
	}
	return true
}
