// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package base defines shared basic pieces of the ent command.
package base

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"github.com/protobuf-orm/ent/cmd/internal/printer"
	"github.com/protobuf-orm/ent/dialect/sql/schema"
	"github.com/protobuf-orm/ent/entc"
	"github.com/protobuf-orm/ent/entc/gen"
	"github.com/protobuf-orm/ent/schema/field"

	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/arg"
	"github.com/lesomnus/xli/flg"
)

// idTypes are the types the --idtype flag accepts.
var idTypes = []field.Type{
	field.TypeInt,
	field.TypeInt64,
	field.TypeUint,
	field.TypeUint64,
	field.TypeString,
}

// parseIDType resolves the --idtype flag value.
func parseIDType(s string) (field.Type, error) {
	for _, t := range idTypes {
		if t.String() == s {
			return t, nil
		}
	}
	return field.TypeInvalid, fmt.Errorf("invalid type %q, expected one of %v", s, idTypes)
}

// InitCmd returns the init command for ent/c packages.
func InitCmd() *xli.Command {
	c := NewCmd()
	c.Name = "init"
	c.Brief = `initialize an environment with zero or more schemas (deprecated: use "new")`
	c.Synop = synop(
		`Deprecated: use "ent new" instead.`,
		"",
		"  ent init Example",
		"  ent init --target entv1/schema User Group",
		"  ent init --template ./path/to/file.tmpl User",
	)
	return c
}

// NewCmd returns the new command for ent/c packages.
func NewCmd() *xli.Command {
	return &xli.Command{
		Name:  "new",
		Brief: "initialize a new environment with zero or more schemas",
		Synop: synop(
			"  ent new Example",
			"  ent new --target entv1/schema User Group",
			"  ent new --template ./path/to/file.tmpl User",
		),
		Flags: flg.Flags{
			&flg.String{Name: "target", Brief: "target directory for schemas", Default: ref(defaultSchema)},
			&flg.String{Name: "template", Brief: "template to use for new schemas"},
		},
		Args: arg.Args{
			&arg.RestStrings{Name: "SCHEMA", Brief: "names of the schemas to create"},
		},
		Handler: xli.OnRun(func(ctx context.Context, cmd *xli.Command, next xli.Next) error {
			names, _ := arg.Get[[]string](cmd, "SCHEMA")
			for _, name := range names {
				if !unicode.IsUpper(rune(name[0])) {
					return errors.New("schema names must begin with uppercase")
				}
			}
			var (
				err         error
				tmpl        *template.Template
				target      = flg.MustGet[string](cmd, "target")
				tmplPath, _ = flg.Get[string](cmd, "template")
			)
			if tmplPath != "" {
				tmpl = template.New(filepath.Base(tmplPath)).Funcs(gen.Funcs)
				tmpl, err = tmpl.ParseFiles(tmplPath)
			} else {
				tmpl = template.New("schema").Funcs(gen.Funcs)
				tmpl, err = tmpl.Parse(defaultTemplate)
			}
			if err != nil {
				return fmt.Errorf("ent/new: could not parse template %w", err)
			}
			if err := newEnv(target, names, tmpl); err != nil {
				return fmt.Errorf("ent/new: %w", err)
			}
			return next(ctx)
		}),
	}
}

// DescribeCmd returns the describe command for ent/c packages.
func DescribeCmd() *xli.Command {
	return &xli.Command{
		Name:  "describe",
		Brief: "print a description of the graph schema",
		Synop: synop(
			"  ent describe ./ent/schema",
			"  ent describe github.com/a8m/x",
		),
		Args: arg.Args{
			&arg.String{Name: "PATH", Brief: "the schema directory or package"},
		},
		Handler: xli.OnRun(func(ctx context.Context, cmd *xli.Command, next xli.Next) error {
			graph, err := entc.LoadGraph(arg.MustGet[string](cmd, "PATH"), &gen.Config{})
			if err != nil {
				return err
			}
			printer.Fprint(os.Stdout, graph)
			return next(ctx)
		}),
	}
}

// GenerateCmd returns the generate command for ent/c packages.
func GenerateCmd(postRun ...func(*gen.Config)) *xli.Command {
	return &xli.Command{
		Name:  "generate",
		Brief: "generate go code for the schema directory",
		Synop: synop(
			"  ent generate ./ent/schema",
			"  ent generate github.com/a8m/x",
		),
		Flags: flg.Flags{
			&flg.String{Name: "storage", Brief: "storage driver to support in codegen", Default: ref("sql")},
			&flg.String{Name: "header", Brief: "override codegen header"},
			&flg.String{Name: "target", Brief: "target directory for codegen"},
			&flg.Strings{Name: "feature", Brief: "extend codegen with additional features"},
			&flg.Strings{Name: "template", Brief: "external templates to execute"},
			// The --idtype flag predates the field.<Type>("id") option.
			// See, https://entgo.io/docs/schema-fields#id-field.
			&flg.String{
				Name:     "idtype",
				Category: "deprecated",
				Brief:    fmt.Sprintf("type of the id field, one of %v", idTypes),
				Default:  ref(field.TypeInt.String()),
			},
		},
		Args: arg.Args{
			&arg.String{Name: "PATH", Brief: "the schema directory or package"},
		},
		Handler: xli.OnRun(func(ctx context.Context, cmd *xli.Command, next xli.Next) error {
			var cfg gen.Config
			cfg.Header, _ = flg.Get[string](cmd, "header")
			cfg.Target, _ = flg.Get[string](cmd, "target")
			features, _ := flg.Get[[]string](cmd, "feature")
			templates, _ := flg.Get[[]string](cmd, "template")

			idtype, err := parseIDType(flg.MustGet[string](cmd, "idtype"))
			if err != nil {
				return err
			}
			opts := []entc.Option{
				entc.Storage(flg.MustGet[string](cmd, "storage")),
				entc.FeatureNames(features...),
			}
			for _, tmpl := range templates {
				typ := "dir"
				if parts := strings.SplitN(tmpl, "=", 2); len(parts) > 1 {
					typ, tmpl = parts[0], parts[1]
				}
				switch typ {
				case "dir":
					opts = append(opts, entc.TemplateDir(tmpl))
				case "file":
					opts = append(opts, entc.TemplateFiles(tmpl))
				case "glob":
					opts = append(opts, entc.TemplateGlob(tmpl))
				default:
					return fmt.Errorf("unsupported template type %q", typ)
				}
			}
			// If the target directory is not inferred from
			// the schema path, resolve its package path.
			if cfg.Target != "" {
				pkgPath, err := PkgPath(DefaultConfig, cfg.Target)
				if err != nil {
					return err
				}
				cfg.Package = pkgPath
			}
			cfg.IDType = &field.TypeInfo{Type: idtype}
			if err := entc.Generate(arg.MustGet[string](cmd, "PATH"), &cfg, opts...); err != nil {
				return err
			}
			for _, fn := range postRun {
				fn(&cfg)
			}
			return next(ctx)
		}),
	}
}

// SchemaCmd returns DDL to use Ent as an Atlas schema loader.
func SchemaCmd() *xli.Command {
	return &xli.Command{
		Name:  "schema",
		Brief: "dump the DDL for the schema directory",
		Synop: synop(
			"  ent schema ./ent/schema --dialect mysql --version 5.6",
			"  ent schema ./ent/schema --dialect sqlite3",
			"  ent schema github.com/a8m/x --dialect postgres --version 15",
		),
		Flags: flg.Flags{
			&flg.String{Name: "dialect", Brief: "database dialect to use", Required: true},
			&flg.String{Name: "version", Brief: "database version to assume"},
			&flg.Strings{Name: "feature", Brief: "extend codegen with additional features"},
			&flg.Strings{Name: "build-tags", Brief: "go build tags to use when loading the schema graph"},
			&flg.Switch{Name: "hash-symbols", Brief: "whether to hash long symbols"},
		},
		Args: arg.Args{
			&arg.String{Name: "PATH", Brief: "the schema directory or package"},
		},
		Handler: xli.OnRun(func(ctx context.Context, cmd *xli.Command, next xli.Next) error {
			var cfg gen.Config
			features, _ := flg.Get[[]string](cmd, "feature")
			buildTags, _ := flg.Get[[]string](cmd, "build-tags")
			for _, o := range []entc.Option{
				entc.FeatureNames(features...),
				entc.BuildTags(buildTags...),
			} {
				if err := o(&cfg); err != nil {
					return err
				}
			}
			g, err := entc.LoadGraph(arg.MustGet[string](cmd, "PATH"), &cfg)
			if err != nil {
				return err
			}
			t, err := g.Tables()
			if err != nil {
				return err
			}
			v, err := g.Views()
			if err != nil {
				return err
			}
			version, _ := flg.Get[string](cmd, "version")
			hashSymbols, _ := flg.Get[bool](cmd, "hash-symbols")
			ddl, err := schema.DDL(ctx, schema.DDLArgs{
				Dialect:     flg.MustGet[string](cmd, "dialect"),
				Version:     version,
				HashSymbols: hashSymbols,
				Tables:      append(t, v...),
			})
			if err != nil {
				return err
			}
			fmt.Println(ddl)
			return next(ctx)
		}),
	}
}

// newEnv create a new environment for ent codegen.
func newEnv(target string, names []string, tmpl *template.Template) error {
	if err := createDir(target); err != nil {
		return fmt.Errorf("create dir %s: %w", target, err)
	}
	for _, name := range names {
		if err := gen.ValidSchemaName(name); err != nil {
			return fmt.Errorf("new schema %s: %w", name, err)
		}
		if fileExists(target, name) {
			return fmt.Errorf("new schema %s: already exists", name)
		}
		b := bytes.NewBuffer(nil)
		if err := tmpl.Execute(b, name); err != nil {
			return fmt.Errorf("executing template %s: %w", name, err)
		}
		newFileTarget := filepath.Join(target, strings.ToLower(name+".go"))
		if err := os.WriteFile(newFileTarget, b.Bytes(), 0644); err != nil {
			return fmt.Errorf("writing file %s: %w", newFileTarget, err)
		}
	}
	return nil
}

func createDir(target string) error {
	_, err := os.Stat(target)
	if err == nil || !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(target, os.ModePerm); err != nil {
		return fmt.Errorf("creating schema directory: %w", err)
	}
	if target != defaultSchema {
		return nil
	}
	if err := os.WriteFile("ent/generate.go", []byte(genFile), 0644); err != nil {
		return fmt.Errorf("creating generate.go file: %w", err)
	}
	return nil
}

func fileExists(target, name string) bool {
	var _, err = os.Stat(filepath.Join(target, strings.ToLower(name+".go")))

	return err == nil
}

const (
	// default schema package path.
	defaultSchema = "ent/schema"
	// ent/generate.go file used for "go generate" command.
	genFile = "package ent\n\n//go:generate go run -mod=mod github.com/protobuf-orm/ent/cmd/ent generate ./schema\n"
	// schema template for the "init" command.
	defaultTemplate = `package schema

import "github.com/protobuf-orm/ent"

// {{ . }} holds the schema definition for the {{ . }} entity.
type {{ . }} struct {
	ent.Schema
}

// Fields of the {{ . }}.
func ({{ . }}) Fields() []ent.Field {
	return nil
}

// Edges of the {{ . }}.
func ({{ . }}) Edges() []ent.Edge {
	return nil
}
`
)

// synop joins the lines of a command description, which the help renders
// under "Description".
func synop(lines ...string) string { return strings.Join(lines, "\n    ") }

// ref returns a pointer to v, which is how a flag carries its default.
func ref[T any](v T) *T { return &v }
