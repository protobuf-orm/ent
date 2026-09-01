// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/protobuf-orm/ent/cmd/internal/base"
	"github.com/protobuf-orm/ent/entc/gen"

	"github.com/lesomnus/xli"
)

func main() {
	log.SetFlags(0)
	cmd := &xli.Command{
		Name:    "entc",
		Brief:   "the ent code generator",
		Handler: xli.RequireSubcommand(),
		Commands: xli.Commands{
			base.NewCmd(),
			base.DescribeCmd(),
			base.GenerateCmd(migrate),
			base.InitCmd(),
		},
	}
	if err := cmd.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func migrate(c *gen.Config) {
	var (
		target = filepath.Join(c.Target, "generate.go")
		oldCmd = []byte("github.com/protobuf-orm/ent/cmd/entc")
	)
	buf, err := os.ReadFile(target)
	if err != nil || !bytes.Contains(buf, oldCmd) {
		return
	}
	_ = os.WriteFile(target, bytes.ReplaceAll(buf, oldCmd, []byte("github.com/protobuf-orm/ent/cmd/ent")), 0644)
}
