// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package printer

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/protobuf-orm/ent/entc/gen"
)

// A Config controls the output of Fprint.
type Config struct {
	io.Writer
}

// Print prints a table description of the graph to the given writer.
func (p Config) Print(g *gen.Graph) {
	for _, n := range g.Nodes {
		p.node(n)
	}
}

// Fprint executes "pretty-printer" on the given writer.
func Fprint(w io.Writer, g *gen.Graph) {
	Config{Writer: w}.Print(g)
}

var (
	fieldHeader = []string{"Field", "Type", "Unique", "Optional", "Nillable", "Default", "UpdateDefault", "Immutable", "StructTag", "Validators", "Comment"}
	edgeHeader  = []string{"Edge", "Type", "Inverse", "BackRef", "Relation", "Unique", "Optional", "Comment"}
)

// node returns description of a type. The format of the description is:
//
//	Type:
//			<Fields Table>
//
//			<Edges Table>
func (p Config) node(t *gen.Type) {
	var (
		b  strings.Builder
		id []*gen.Field
	)
	b.WriteString(t.Name + ":\n")
	if t.ID != nil {
		id = append(id, t.ID)
	}
	fields := make([][]string, 0, len(id)+len(t.Fields))
	for _, f := range append(id, t.Fields...) {
		v := reflect.ValueOf(*f)
		row := make([]string, len(fieldHeader))
		for i := 0; i < len(row)-1; i++ {
			field := v.FieldByNameFunc(func(name string) bool {
				// The first field is mapped from "Name" to "Field".
				return name == "Name" && i == 0 || name == fieldHeader[i]
			})
			row[i] = fmt.Sprint(field.Interface())
		}
		row[len(row)-1] = f.Comment()
		fields = append(fields, row)
	}
	table(&b, fieldHeader, fields)
	edges := make([][]string, 0, len(t.Edges))
	for _, e := range t.Edges {
		edges = append(edges, []string{
			e.Name,
			e.Type.Name,
			strconv.FormatBool(e.IsInverse()),
			e.Inverse,
			e.Rel.Type.String(),
			strconv.FormatBool(e.Unique),
			strconv.FormatBool(e.Optional),
			e.Comment(),
		})
	}
	if len(edges) > 0 {
		// Without the borders the previous renderer drew, the two tables
		// need a blank line to be told apart.
		b.WriteString("\n")
		table(&b, edgeHeader, edges)
	}
	io.WriteString(p, strings.ReplaceAll(b.String(), "\n", "\n\t")+"\n")
}

// table writes the header and rows as a column-aligned table. Empty trailing
// cells are padded like any other, so the result is trimmed line by line to
// keep the output free of trailing whitespace.
func table(w io.Writer, header []string, rows [][]string) {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	for _, cells := range append([][]string{header}, rows...) {
		io.WriteString(tw, strings.Join(cells, "\t")+"\n")
	}
	tw.Flush()
	for _, line := range strings.SplitAfter(b.String(), "\n") {
		if line == "" {
			continue
		}
		io.WriteString(w, strings.TrimRight(line, " \t\n")+"\n")
	}
}
