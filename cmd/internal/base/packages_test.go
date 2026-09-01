// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package base

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/stretchr/testify/require"
)

func TestPkgPath(t *testing.T) {
	// A module named golang.org/x holding a package of its own and a nested one.
	dir := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(contents), 0644))
	}
	write("go.mod", "module golang.org/x\n\ngo 1.27\n")
	write("x.go", "package x\n")
	write("y/y.go", "package y\n")

	cfg := &packages.Config{
		Mode: packages.NeedName,
		// The module has no requirements, so nothing should be fetched.
		Env: append(os.Environ(), "GOPROXY=off", "GOFLAGS=-mod=mod"),
	}

	// A target inside a package resolves against that package.
	cfg.Dir = dir
	pkgPath, err := PkgPath(cfg, filepath.Join(dir, "ent"))
	require.NoError(t, err)
	require.Equal(t, "golang.org/x/ent", pkgPath)

	// And so does one inside a nested package.
	cfg.Dir = filepath.Join(dir, "y")
	pkgPath, err = PkgPath(cfg, filepath.Join(cfg.Dir, "ent"))
	require.NoError(t, err)
	require.Equal(t, "golang.org/x/y/ent", pkgPath)

	// Two directories that do not exist yet are still resolved.
	pkgPath, err = PkgPath(cfg, filepath.Join(cfg.Dir, "z/ent"))
	require.NoError(t, err)
	require.Equal(t, "golang.org/x/y/z/ent", pkgPath)

	// Three are more than PkgPath walks up.
	pkgPath, err = PkgPath(cfg, filepath.Join(cfg.Dir, "z/e/n/t"))
	require.Error(t, err)
	require.Empty(t, pkgPath)
}
