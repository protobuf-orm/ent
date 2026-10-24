// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package field_test

import (
	"bufio"
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMakingFieldsLetsTheLinkerPruneMethods builds testdata/linked, which makes
// a schema's fields as a generated client's runtime.go does at init, and reads
// what the linker kept of it.
//
// A function that calls reflect.Type.Method, or MethodByName with a name the
// compiler cannot see, is marked for the linker, and a program that reaches
// one keeps every exported method of every type converted to an interface,
// since any of them might now be called by name. Nothing fails: the program
// works, and is larger. Listing a field's methods as its descriptor was made
// did it to every program with a generated client, and that was 24 MB of the
// 63 MB of lesomnus/payday's app compiled to WebAssembly (RType.LoadMethods).
// The linker's dump (-dumpdep) names each marked function it reaches.
func TestMakingFieldsLetsTheLinkerPruneMethods(t *testing.T) {
	t.Run("making fields", func(t *testing.T) {
		marked, chain := linked(t)
		require.Empty(t, marked, "so the linker keeps every method; it reached the first through\n\t%s", chain)
	})

	// The loader lists methods on purpose, so the program built with what it
	// does calls one by index -- and has to be seen to, or a dump that could
	// not show it would pass anything above.
	t.Run("and loading them", func(t *testing.T) {
		marked, _ := linked(t, "load")
		require.Contains(t, marked, "github.com/protobuf-orm/ent/schema/field.(*RType).LoadMethods")
	})
}

// linked builds testdata/linked with tags, and answers the functions the linker
// reached that are marked for calling a method by index, with how it reached
// the first of them.
func linked(t *testing.T, tags ...string) (marked []string, chain string) {
	t.Helper()

	// The dump is the linker's, which the go command passes on with the rest
	// of what the linker says, on stderr.
	c := exec.CommandContext(t.Context(), "go", "build",
		"-tags", strings.Join(tags, ","),
		"-ldflags=-dumpdep",
		"-o", filepath.Join(t.TempDir(), "linked"),
		"./testdata/linked",
	)
	var dump bytes.Buffer
	c.Stdout = &dump
	c.Stderr = &dump
	require.NoError(t, c.Run(), "%s", dump.String())

	// A line is `from -> to`, written when `to` is first reached, each name
	// followed by what the linker marked it with.
	from := map[string]string{}
	s := bufio.NewScanner(&dump)
	s.Buffer(nil, 1<<20)
	for s.Scan() {
		a, b, ok := strings.Cut(s.Text(), " -> ")
		if !ok {
			continue
		}
		a, _ = symbol(a)
		b, m := symbol(b)
		if _, ok := from[b]; ok {
			continue
		}
		from[b] = a
		if m {
			marked = append(marked, b)
		}
	}
	require.NoError(t, s.Err())
	require.NotEmpty(t, from, "no dump in\n%s", dump.String())

	if len(marked) > 0 {
		names := []string{marked[0]}
		for n := marked[0]; from[n] != "" && from[n] != "_"; n = from[n] {
			names = append(names, from[n])
		}
		for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
			names[i], names[j] = names[j], names[i]
		}
		chain = strings.Join(names, "\n\t")
	}
	return marked, chain
}

// symbol is a name in the dump without what the linker marked it with, and
// whether that was a method called by index.
func symbol(s string) (string, bool) {
	s, m := strings.CutSuffix(s, "<ReflectMethod>")
	s = strings.TrimSuffix(s, "<UsedInIface>")
	return strings.TrimSuffix(s, " "), m
}
