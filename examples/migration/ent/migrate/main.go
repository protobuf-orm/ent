// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

//go:build ignore

package main

import (
	"context"
	"log"
	"os"

	atlas "ariga.io/atlas/sql/migrate"
	_ "github.com/go-sql-driver/mysql"
	"github.com/protobuf-orm/ent/dialect"
	"github.com/protobuf-orm/ent/dialect/sql/schema"
	"github.com/protobuf-orm/ent/examples/migration/ent/migrate"
)

func main() {
	ctx := context.Background()
	dir, err := atlas.NewLocalDir("ent/migrate/migrations")
	if err != nil {
		log.Fatalf("failed creating atlas migration directory: %v", err)
	}
	// Migrate diff options.
	opts := []schema.MigrateOption{
		schema.WithDir(dir),                          // provide migration directory
		schema.WithMigrationMode(schema.ModeReplay),  // provide migration mode
		schema.WithDialect(dialect.MySql),            // Ent dialect to use
		schema.WithFormatter(atlas.DefaultFormatter), // Default Atlas formatter
	}
	if len(os.Args) != 2 {
		log.Fatalln("migration name is required. Use: 'go run -mod=mod ./ent/migrate/main.go <name>'")
	}
	// Generate migrations using Atlas support for MySql (note the Ent dialect option passed above).
	err = migrate.NamedDiff(ctx, "mysql://root:pass@localhost:3306/test", os.Args[1], opts...)
	if err != nil {
		log.Fatalf("failed generating migration file: %v", err)
	}
}
