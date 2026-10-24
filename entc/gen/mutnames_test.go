// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/protobuf-orm/ent/entc/load"
	"github.com/protobuf-orm/ent/schema/field"
	"github.com/stretchr/testify/require"
)

// fieldMethods are the names of a field's own mutation methods.
var fieldMethods = map[string]func(Field) string{
	"MutationGet":      Field.MutationGet,
	"MutationGetOld":   Field.MutationGetOld,
	"MutationSet":      Field.MutationSet,
	"MutationReset":    Field.MutationReset,
	"MutationClear":    Field.MutationClear,
	"MutationCleared":  Field.MutationCleared,
	"MutationAdd":      Field.MutationAdd,
	"MutationAdded":    Field.MutationAdded,
	"MutationAppend":   Field.MutationAppend,
	"MutationAppended": Field.MutationAppended,
}

// TestField_MutationNames names a field after each method every mutation has,
// and after what is left of one once a field method's prefix or suffix is
// taken off it -- `field` for AddField, `edge` for EdgeCleared -- and requires
// that none of that field's own methods is one of them.
//
// One that is would be declared twice, or would be shadowed by the ent
// package's mutation, and the client would not compile: the field method and
// the generic one taking a field by its name are different functions of the
// same name. Two namers did not ask (Clear, Cleared), and the dispatchers
// that call a field's method by name called the generic one instead.
func TestField_MutationNames(t *testing.T) {
	typ := &Type{Id: &Field{Name: "id", Type: &field.TypeInfo{Type: field.TypeInt}}}
	typ.Id.typ = typ

	affixes := [][2]string{
		{"", ""}, {"Old", ""}, {"Set", ""}, {"Reset", ""}, {"Clear", ""}, {"", "Cleared"},
		{"Add", ""}, {"Added", ""}, {"Append", ""}, {"Appended", ""},
	}
	tried := map[string]bool{}
	for m := range mutMethods {
		for _, a := range affixes {
			rest, ok := strings.CutPrefix(m, a[0])
			if !ok {
				continue
			}
			if rest, ok = strings.CutSuffix(rest, a[1]); !ok || rest == "" || !unicode.IsUpper(rune(rest[0])) {
				continue
			}
			name := snakeWords(rest)
			if pascal(name) != rest || tried[name] {
				continue
			}
			tried[name] = true

			f := Field{Name: name, typ: typ, Type: &field.TypeInfo{Type: field.TypeInt}}
			for namer, method := range fieldMethods {
				got := method(f)
				require.False(t, mutMethods[got], "%s of a field named %q is %s, which every mutation has", namer, name, got)
			}
		}
	}
	for _, name := range []string{"field", "fields", "edge", "ids", "where", "op", "type"} {
		require.True(t, tried[name], "%q was not tried", name)
	}
}

// snakeWords is a name whose words are the PascalCase words of s, which is
// pascal undone: WhereP is where_p.
func snakeWords(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// TestMutationMethods generates a type with nothing of its own and requires
// that every exported method its mutations have, in the type's package and in
// the ent package, is one the field namers keep clear of.
//
// mutMethods is ent.Mutation's methods and a list of the others, and the list
// is the part that can go stale: it missed Ids, which a field named `ids`
// shadowed, since this fork spells upstream's IDs as a word.
func TestMutationMethods(t *testing.T) {
	target := filepath.Join(t.TempDir(), "ent")
	graph, err := NewGraph(&Config{
		Package: "entc/gen",
		Target:  target,
		Storage: drivers[0],
		IdType:  &field.TypeInfo{Type: field.TypeInt},
	}, &load.Schema{Name: "T1"})
	require.NoError(t, err)
	require.NoError(t, graph.Gen())

	methods := map[string]bool{}
	for path, receiver := range map[string]string{
		filepath.Join(target, "t1", "mutation.go"): "Mutation",
		filepath.Join(target, "mutation.go"):       "T1Mutation",
	} {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() || named(fn.Recv.List[0].Type) != receiver {
				continue
			}
			methods[fn.Name.Name] = true
		}
	}
	require.Contains(t, methods, "Ids", "the generated mutation was not read")
	for m := range methods {
		// The id's own getter, which no other field can be named after.
		if m == "Id" {
			continue
		}
		require.True(t, mutMethods[m], "every mutation has %s, and a field may be given a method of that name", m)
	}
}

// named is the name of a receiver's type, through its pointer.
func named(e ast.Expr) string {
	if s, ok := e.(*ast.StarExpr); ok {
		e = s.X
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
