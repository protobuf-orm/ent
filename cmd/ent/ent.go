// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/protobuf-orm/ent/cmd/internal/base"

	"github.com/lesomnus/xli"
)

func main() {
	log.SetFlags(0)
	cmd := &xli.Command{
		Name:    "ent",
		Brief:   "the ent code generator",
		Handler: xli.RequireSubcommand(),
		Commands: xli.Commands{
			base.NewCmd(),
			base.DescribeCmd(),
			base.GenerateCmd(),
			base.InitCmd(),
			base.SchemaCmd(),
		},
	}
	if err := cmd.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
