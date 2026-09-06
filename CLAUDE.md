# Working in this repository

This is a **fork** of [ent/ent](https://github.com/ent/ent), not a soft one.
Names, dependencies and several behaviours differ on purpose.

**Before merging, cherry-picking or hand-porting anything from upstream, read
[UPSTREAM.md](UPSTREAM.md).** It lists every deliberate divergence and what to
do with each. Most upstream conflicts are this fork's own renames sitting next
to the change, not real disagreements — resolving them by reading each hunk
from scratch wastes a lot of time.

## Naming

`pascal()` capitalizes the first letter of each word and consults no
dictionary. `user_id` is `UserId`, not `UserID`; `api_key` is `ApiKey`.

Two exceptions, both easy to get wrong:

- **A name that belongs to another package keeps that package's spelling** —
  `uuid.UUID`, `url.URL`, `net.IP`, everything under `ariga.io/atlas`. A name
  derived from one keeps it too: `[]*url.URL` is still `URLs`.
- **A method answering an interface is spelled by the interface** —
  `MarshalJSON`, `UnmarshalJSON`, `Value`, `Scan`. Renaming `MarshalJSON` to
  match the rule stops the type being a `json.Marshaler`, and `encoding/json`
  falls back to reflection **without an error**.

Prose says UUID, SQL, JSON and ID as English words. Only identifiers changed.

Table names are the snake_case entity name, **not pluralized**: `User` → `user`.

## Modules

Four. `cmd` is part of the root module; the other three are separate.

```
.                                   root
examples/
dialect/sql/schema/integration/     SQLite only, no services
entc/integration/                   the engine matrix
```

## Testing

```sh
go test ./...
(cd examples && go test ./...)
(cd dialect/sql/schema/integration && go test ./...)

docker compose -f compose.test.yaml up -d --wait
(cd entc/integration && go test -race -count=2 ./...)
docker compose -f compose.test.yaml down
```

The engines are not optional. SQLite alone has hidden real bugs here — see
`d292c1129`, where every UUID insert on MySql failed while SQLite was green.

A new engine-level test goes in `entc/integration/<topic>/`; the CI job picks
it up with no workflow change. Small driver-level packages with no codegen are
fine — see `enttime` and `entvar`. Give a package that creates tables its own
database: packages there run in parallel against one server, twice each.

## The generate gate

CI fails if the tree is dirty after generating. The order matters: `go generate`
runs `go run -mod=mod .../cmd/ent`, which *adds* entries to
`entc/integration/go.sum` that `go mod tidy` then removes. Commit the tidied
state, not the intermediate one.

```sh
go generate ./... && go mod tidy \
  && (cd examples && go generate ./... && go mod tidy) \
  && (cd entc/integration && go generate ./... && go mod tidy) \
  && git status --porcelain   # must print nothing
```

Never resolve a conflict in generated code by hand — regenerate. Anything under
`*/ent/` or a `migrate/schema.go` is output.

`go vet ./dialect/sql/` reports a pre-existing `WriteByte` signature complaint
in `builder.go`. That one is expected.

## Commit messages

Look at `git log` before writing one. They are prose: what was wrong, why the
change is the answer, and what it costs — not a summary of the diff.
