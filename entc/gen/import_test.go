// Copyright 2026 Seunghyun Hwang. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"database/sql"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/protobuf-orm/ent/dialect/entsql"
	"github.com/protobuf-orm/ent/entc/load"
	"github.com/protobuf-orm/ent/schema/field"
	"github.com/stretchr/testify/require"
)

// TestGraph_Gen_imports generates a graph and fails if goimports had to find
// a package that is not in the standard library.
//
// That is a package the templates use and leave out of a file's imports, and
// goimports resolves the name by whatever package of that name it finds,
// which need not be the one meant: a generated client's `ent` was once
// another generated package's, since every one of them exports Hook and
// Interceptor. The standard library is a fixed table, so leaving a name in it
// to goimports is safe.
//
// The graph is generated into a module of its own that requires this one, so
// that goimports can find what a template leaves out, and does.
func TestGraph_Gen_imports(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(
		"module example.com/gen\n\ngo 1.27\n\nrequire github.com/protobuf-orm/ent v0.0.0\n\n"+
			"replace github.com/protobuf-orm/ent => "+root+"\n",
	), 0644))

	desc := func(f interface{ Descriptor() *field.Descriptor }) *load.Field {
		lf, err := load.NewField(f.Descriptor())
		require.NoError(t, err)
		// Where runtime.go finds a default: the schema's own field.
		lf.Position = &load.Position{}
		return lf
	}
	schemas := []*load.Schema{
		{
			// An Id of a type outside the standard library, and a field of a
			// type whose package shares its name with a predefined import.
			Name: "T1",
			Fields: []*load.Field{
				desc(field.String("id").GoType(entsql.ReferenceOption(""))),
				desc(field.String("str").GoType(sql.NullString{}).
					DefaultFunc(func() sql.NullString { return sql.NullString{} })),
			},
			Edges: []*load.Edge{{Name: "t2", Type: "T2", Unique: true}},
		},
		{
			Name:   "T2",
			Fields: []*load.Field{desc(field.Uuid("id"))},
		},
	}
	graph, err := NewGraph(&Config{
		Package:  "example.com/gen/ent",
		Schema:   "example.com/gen/ent/schema",
		Target:   filepath.Join(dir, "ent"),
		Storage:  drivers[0],
		IdType:   &field.TypeInfo{Type: field.TypeInt},
		Features: AllFeatures,
	}, schemas...)
	require.NoError(t, err)

	var (
		mu    sync.Mutex
		found []string
	)
	formatted = func(path string, src, out []byte) {
		before, after := importPaths(t, path, src), importPaths(t, path, out)
		for p := range after {
			if before[p] || !strings.Contains(strings.Split(p, "/")[0], ".") {
				continue
			}
			rel, _ := filepath.Rel(dir, path)
			mu.Lock()
			found = append(found, rel+": "+p)
			mu.Unlock()
		}
	}
	t.Cleanup(func() { formatted = nil })

	require.NoError(t, graph.Gen())
	require.Empty(t, found, "imports the templates left for goimports to find")
}

func importPaths(t *testing.T, path string, src []byte) map[string]bool {
	f, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ImportsOnly)
	require.NoError(t, err)
	paths := make(map[string]bool, len(f.Imports))
	for _, spec := range f.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		require.NoError(t, err)
		paths[p] = true
	}
	return paths
}
