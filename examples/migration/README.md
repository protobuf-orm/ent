# Versioned Migration Example

A schema change becomes a file that is reviewed, committed and applied, rather
than something a server does to a database on the way up.

`example_test.go` runs the whole cycle against SQLite. Everything it calls is
`dialect/sql/schema`, on the atlas packages ent already depends on -- there is
no CLI in it.

### The three calls

```go
d, _ := entschema.OpenDir("migrations")
m := entschema.Migrations{Dir: d, Dialect: dialect.SQLite, Tables: migrate.Tables}
```

**Plan** turns a change of the ent schema into a file of SQL to review. `dev` is
an empty database of the same kind, onto which the files already written are
replayed to work out the state they reach; it is written to and emptied again,
so it must not be one anyone cares about.

```go
files, err := m.Plan(ctx, dev, "add_user_tags")
```

**Apply** is the other command rather than the next step. It runs every file
this database has not run yet, in order, and records each only once all of its
statements have. Having nothing to do is not an error -- a deployment that
applies on every start is up to date almost every time.

```go
files, err := m.Apply(ctx, db, dialect.SQLite)
```

**Current** asks whether every file has run, which is the question a deployment
can answer on the way up. `Check` asks the other one -- whether the database
holds what the ent schema says -- for a deployment that keeps no files.

```go
if err := m.Current(ctx, db, dialect.SQLite); err != nil {
	return err // started against a database a migration has not reached
}
```

### Where the commands live

On **your** binary, not on `ent`: they have to link your ent schema, and a
deployment should run its migration with the same image it serves with.

### Changing the schema

1. Change `ent/schema`.
2. `go generate ./ent`
3. Plan a file, read it, commit it.

`OpenDir` refuses a directory whose files were edited after they were written,
which is what `atlas.sum` records. A migration that has run somewhere is not a
file to fix in place.
