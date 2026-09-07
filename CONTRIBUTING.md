# Contributing to ent

**Bug reports are welcome. Code is not.**

Pull requests are restricted to collaborators, so there is no way to send one by
accident. Please do not paste a patch into an issue either, and do not describe
a fix in code.

I want this project to remain something I can design and maintain end to end.
Accepting outside code means accepting the design decisions that came with it
and the maintenance that follows, and that is not what I want for this one.

It is not a judgement about anyone's code, and it is written down because a
policy nobody states is one people run into.

Nothing about that restricts what you may do with what is here. It is Apache
2.0, and a release made under those terms stays available under them. This is a
fork of [ent/ent](https://github.com/ent/ent) and carries their copyright as
well; see [NOTICE](NOTICE).

## Reporting a problem

**Describe the problem, not the fix.** Open an issue with what you did, what you
expected and what happened; the smallest schema that shows it; and versions --
this module, Go, and the database. If you have worked out why something breaks,
say why in prose.

Report a security problem privately rather than in an issue: use GitHub's
security advisory form on this repository, or email the address on the commits.

## Working on this fork

Before merging, cherry-picking or hand-porting anything from upstream, read
[UPSTREAM.md](UPSTREAM.md): it lists what this fork changed and what to drop,
rewrite or regenerate when a patch does not apply.

# Project structure

- `dialect` - Contains the SQL code used by the generated code.
  - `dialect/sql/schema` - Auto migration logic resides there.
  - `dialect/sql/schema/integration` - A module of its own, holding the migration
    tests that run against a real database engine so that their driver stays out
    of the go.mod of ent.
  - `dialect/sql/sqljson` - JSON extension for SQL.

- `schema` - User schema API.
  - `schema/{field, edge, index, mixin}` - provides schema builders API.
  - `schema/field/gen` - Templates and codegen for numeric builders.

- `entc` - Codegen of `ent`.
  - `entc/load` - `entc` loader API for loading user schemas into a Go objects at runtime.
  - `entc/gen` - The actual code generation logic resides in this package (and its `templates` package).
  - `integration` - Integration tests for `entc`.

- `privacy` - Runtime code for [privacy layer](https://entgo.io/docs/privacy/).

- `doc/md` - Markdown files for documentation.

# Run integration tests
If you touch any file in `entc`, regenerate in `entc/integration` and `examples`:

```
go generate ./...
go mod tidy
```

The engines the suite talks to are in `compose.test.yaml` at the root -- the
same six the workflow declares, on the ports the tests look for:

```
docker compose -f compose.test.yaml up -d --wait
(cd entc/integration && go test -race -count=2 ./...)
docker compose -f compose.test.yaml down
```


## License
Apache 2.0, in the LICENSE file at the root. [NOTICE](NOTICE) records that this
is a fork and what came from where.
