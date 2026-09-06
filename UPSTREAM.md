# Taking a change from upstream

This is a fork of [ent/ent](https://github.com/ent/ent). It is not a soft fork:
names, dependencies and a few behaviours differ deliberately, and every one of
those differences is a place where an upstream patch will not apply. This file
says what the differences are and what to do with each, so that bringing a
change in is a procedure rather than an investigation.

Read it before merging, cherry-picking or hand-porting anything from upstream.

## Why the fork exists

Three reasons, in the order they cost:

1. **Codegen had to be predictable.** Upstream worked names out by consulting
   dictionaries -- an inflector for table names, a list of forty-four
   initialisms for identifier case. What a schema produced could not be derived
   from the schema. Both dictionaries are gone; names follow rules that depend
   on the name alone.
2. **The dependency graph had to be small.** This module is a tool in other
   modules' graphs, so a dependency here is a dependency there. Several were
   dropped or swapped for what the rest of that stack already uses.
3. **A few things had to exist that upstream does not offer** -- an external
   codec on a JSON column, a client that can say what it runs on, a
   transaction that several clients can share.

## The procedure

```
git fetch --no-tags https://github.com/ent/ent.git master
git checkout -b upstream-<topic>
git merge FETCH_HEAD          # or: git cherry-pick <sha>
```

Then, in this order:

1. **Delete what we do not have.** `DU` conflicts (deleted by us, modified by
   them) are almost always Gremlin or another removed tree -- `git rm` them
   without reading. See [Removed](#1-removed-drop-the-hunk).
2. **Keep our dependency files.** `go.mod` / `go.sum` conflicts are the merge
   trying to restore what we dropped. Take ours, then let `go mod tidy` decide.
   See [Dependencies](#3-dependencies-keep-ours).
3. **Throw away generated code and regenerate.** Never resolve a conflict in a
   generated file by hand. See [Generated code](#5-generated-code-never-resolve-by-hand).
4. **Rewrite names.** This is the bulk of the work and it is mechanical. See
   [Names](#2-names-rewrite-mechanically).
5. **Read the behaviour deltas.** These are the only conflicts that need
   thought. See [Behaviour](#4-behaviour-read-before-resolving).
6. **Take none of their workflow.** `.github/` is ours, and one of their jobs
   cannot pass here whatever it is named. See [CI](#6-ci-ours-not-theirs).
7. **Verify.** See [Verifying](#verifying).

`git status --porcelain` after a merge sorts the work for you:

| Code | Meaning | Usually |
|---|---|---|
| `DU` | we deleted, they changed | `git rm` -- a removed tree |
| `UA` | they added, we have not | often rename-detection noise; check it exists upstream at all |
| `UU` | both changed | names, deps, generated code, or a real delta |

Rename detection matches generated ent code against other generated ent code,
so a `UA` list can name files that exist in neither tree. Check with
`git cat-file -e FETCH_HEAD:<path>` before believing it.

---

## 1. Removed: drop the hunk

Nothing upstream writes for these is wanted. Delete the files, delete the
template blocks, delete the references.

| Removed | Commit | Note |
|---|---|---|
| **Gremlin storage driver** | `1cec66208` | `dialect/gremlin`, `entc/gen/template/dialect/gremlin`, `entc/integration/gremlin`, `entc/integration/compose`. SQL is the only backend. A feature that touches storage will have a gremlin half -- drop it. |
| **`cmd/entfix`** | `cd9decb57` | The global-id conversion tool. |
| **The acronym dictionary** | `4965c66b7` | `AddAcronym` survives as something a caller *says*; the built-in list is empty. |
| **Inflection** | `43618422d`, `9a09df53e` | No pluralize/singularize/camelize anywhere. |
| **all-contributors** | `ab917d4a1` | |
| **Upstream CI and the doc site** | `20f781224`, `fdd769d62` | See [CI](#6-ci-ours-not-theirs) -- one of their jobs cannot pass here by construction. `doc/md/` is kept as reference prose, not as a published site. |

## 2. Names: rewrite mechanically

**This is where most conflicts come from, and none of them are interesting.**
Upstream's hunk usually differs from ours only in the spelling of an identifier
that happens to sit next to the change.

### The rule

`pascal()` capitalizes the first letter of each word and nothing else. A name
is worked out from the name -- no lookup.

```
user_id   => UserId     (not UserID)
api_key   => ApiKey     (not APIKey)
http_code => HttpCode
```

### Exceptions -- names that belong to somebody else

A name that is not ours keeps the spelling its own author chose. This covers:

- **Types from other packages**: `uuid.UUID`, `url.URL`, `net.IP`, everything
  under `ariga.io/atlas`. A name derived from such a type keeps it too --
  a dependency on `[]*url.URL` is still `URLs`.
- **Methods that answer to an interface**: `MarshalJSON`, `UnmarshalJSON`,
  `Value`, `Scan`. These are spelled by the package that declares the
  interface. Getting this wrong is silent: a `json.Marshaler` renamed to
  `MarshalJson` is not a `json.Marshaler`, and `encoding/json` falls back to
  reflection without complaining (`27a8e3fa9`).
- **Prose.** Comments say UUID, SQL, JSON, ID as English words. Only
  identifiers change -- a word in backticks, in a doc link, or naming something
  the file declares is left as the identifier it is (`1d321ed5f`).

### The ones you will actually hit

| Upstream | Here |
|---|---|
| `ID`, `IDs` as a name part | `Id`, `Ids` -- `FieldID`→`FieldId`, `AddedIDs`→`AddedIds`, `$.ID`→`$.Id`, `$.HasCompositeID`→`$.HasCompositeId`, `$.HasOneFieldID`→`$.HasOneFieldId`, `$.EdgesWithID`→`$.EdgesWithId`, `sqlgraph.NodeSpec.ID`→`.Id`, `ent.OpQueryIDs`→`OpQueryIds` |
| `dialect.MySQL` | `dialect.MySql` |
| `field.UUID`, `field.TypeUUID` | `field.Uuid`, `field.TypeUuid` |
| `field.JSON`, `field.TypeJSON` | `field.Json`, `field.TypeJson` |
| `entgo.io/ent` | `github.com/protobuf-orm/ent` (`6f0bc7e08`) |

`dialect.SQLite` and `dialect.Postgres` are unchanged.

Treat that table as a starting point, not as the set. After rewriting, build --
the compiler finds the rest.

### Table and API names

Derived from the entity name alone (`43618422d`). An upstream test fixture or
doc that says `users` means `user` here.

- Table: snake_case entity name, **not pluralized**. `User` → `user`,
  `UserInfo` → `user_info`, `HTTPCode` → `http_code`.
- Parsable slice type: `<Type>List`. `Users` → `UserList`, `People` →
  `PersonList`.
- Edge id mutations keep the edge name as written: `AddGroupIDs` →
  `AddGroupsIds`.
- The disambiguating column of a self-referencing M2M keeps the edge name and
  is snake_cased: edge `bestFriends` → `best_friends_id`.

## 3. Dependencies: keep ours

The root module requires no database driver at all. A merge will try to put
several dependencies back; it is always wrong.

| Upstream uses | We use | Commit |
|---|---|---|
| `github.com/spf13/cobra` | `github.com/lesomnus/xli` | `c97834a23` |
| `github.com/google/uuid` | standard library `uuid` | `4115dcfc4`, `31be5d291` |
| `github.com/mattn/go-sqlite3` | `github.com/ncruces/go-sqlite3` (wasm, no cgo) | `97f8cc5d8` |
| `github.com/go-openapi/inflect` | nothing (indirect via atlas only) | `9a09df53e` |
| `github.com/olekukonko/tablewriter` | `text/tabwriter` | `eca55ff7f` |
| `golang.org/x/tools/go/packages/packagestest` | a fixture built by hand | `88208a9bf` |
| `github.com/json-iterator/go`, `gorilla/websocket`, `go.opencensus.io`, `mitchellh/mapstructure` | nothing (they came with Gremlin) | `1cec66208` |

Go version is **1.27** (`26dcc1103`).

**Resolve every `go.mod`/`go.sum` conflict by taking ours, then running
`go mod tidy`** in the root, `examples/` and `entc/integration/`. If an upstream
feature genuinely needs a new dependency, that is a decision to make on
purpose, not something to let a merge do.

## 4. Behaviour: read before resolving

These are the only conflicts worth slowing down for. An upstream change that
touches the same area may be assuming the behaviour we replaced.

| Area | What differs | Commit |
|---|---|---|
| **UUID storage** | `database/sql` reads and writes a `uuid.UUID` itself; there is no codec. `dialect/sql` converts a `uuid.UUID` argument to canonical text at bind time, because go-sql-driver's own converter refuses the type. | `9b0c180f3`, `d292c1129` |
| **UUID field type** | `field.Uuid(name)` takes the Go type *optionally* -- a UUID field is `uuid.UUID` unless it says otherwise. | `661fec4c6`, `1c09a6cb7` |
| **JSON columns** | A JSON field accepts an external `ValueScanner`. Upstream refuses one. | `fc2386a28` |
| **Client surface** | Generated clients expose `Dialect()`, `Driver()`, `WithDriver(drv)`, `InTx()`. | `d7f47a8b8` |
| **Transactions** | `dialect.Tx` is public enough that several clients can share one transaction; there is a single answer to "am I in a transaction" that sees through a `DebugDriver`. | `78a935fbe`, `855799324` |
| **Paging** | `dialect/sql/sqlpage` -- keyset cursors -- is ours. | `7714d7267` |
| **Time: zone** | Every `time.Time` is UTC at both edges. `bindArgs` moves an argument to UTC; `Rows.Scan` moves a scanned value to UTC; `field.Time`'s `Default`/`UpdateDefault` answer in UTC. | `7d10ad284`, `c6003ab94`, `1d1a9d88b` |
| **Time: precision** | Time columns keep six fractional-second digits by default; `field.Time().Precision(n)` says otherwise. | `cede1c296` |
| **Time: MySql column** | A time field is a `datetime`, not a `timestamp` -- no session `time_zone` conversion, no 2038. | `5d232985b` |
| **Session variables** | A MySql system variable is reset with `DEFAULT`, a user-defined one with `NULL`. | `f3f5b8f55` |
| **SQLite** | `sqlite_sequence` is quoted as a string and the rewrite is narrowed. | `14f94eda3`, `e540949fe` |

Two of these bite hardest when merging a feature that adds a field type:

- **`schema/field.Descriptor`** carries `Precision *int` that upstream does not.
  A merge conflict here is adjacency -- keep both sides.
- **`entc/gen.Field.Column()`** sets `c.Precision`. Same.

## 5. Generated code: never resolve by hand

Anything under `entc/integration/*/ent/`, `examples/*/ent/` or a `migrate/schema.go`
is output. Resolving it by hand produces a file that the next `go generate`
overwrites, and hides whether the templates are actually right.

```sh
git checkout --ours -- <generated paths>   # or --theirs; it does not matter
go generate ./... && go mod tidy
(cd examples && go generate ./... && go mod tidy)
(cd entc/integration && go generate ./... && go mod tidy)
git add -A
```

If the regenerated output still differs from what the feature needs, the bug is
in a template, not in the merge.

## 6. CI: ours, not theirs

`.github/workflows/ci.yml` is ours. Take none of upstream's, and in particular
do not restore the job below on the strength of its name.

### The `migration` job does not test migrations

Upstream runs a job called `migration` on every pull request. It is not about
`entc/integration/migrate` or the `schema` package. It brings up fourteen
engines, checks out `origin/master`, runs the whole integration suite to leave
schemas behind, checks out the PR, and **runs the same suite again against the
same databases**. What it asserts is that upgrading ent does not break a schema
an earlier ent created.

That assertion is one this fork gave up on purpose. Table names are the
snake_case entity name and are not pluralized (`43618422d`), so a schema
`origin/master` creates and one this code creates do not agree about the name
of a single table. The job cannot pass, and dropping it was the point rather
than an oversight (`fdd769d62`).

Same commit, separately: the engine matrix went from fourteen servers to six.
MySQL 5.6 and 5.7, MariaDB 10.2 to 10.4 and PostgreSQL 10 to 12 are versions
upstream itself no longer supports, and eight PostgreSQL versions were eight
runs of one code path. Six is also a number a desk can run, which is what makes
a failure something you find before pushing.

`20f781224` removed two more: `atlas-ci-public.yaml`, which needs an Ariga
Atlas Cloud token, and `dependabot.yml`.

### The migration tests themselves are all still here

Nothing about migration was dropped -- only the upgrade-compatibility job. Do
not "restore" these; they already run.

| Tests | Where they run |
|---|---|
| `entc/integration/migrate` -- `V1ToV2`, check constraints, DB-side defaults, time precision, `Versioned`, `ConsistentVersioned` | the `integration` job, via `go test ./...`; MySql 8/8.4 and Postgres 14/17 |
| `dialect/sql/schema/integration` -- `TestMigrate_Diff`, `TestAtlas_StateReader`, `TestAtlas_ParallelCreate` | the `unit` job, SQLite |

### The Atlas CLI cannot read this fork

`atlas migrate diff --to ent://<pkg>` -- the documented way to generate a
migration directory from an ent schema -- shells out to `entgo.io/ent/cmd/ent`.
That module path is not ours, so the CLI reports "no schema found" for every
schema in this repository. Nothing that depends on it can work here, and
nothing should be written that does.

`entc/integration/multischema` used to. It applied a checked-in migration
directory with `atlas migrate apply` and then queried it, and the directory was
generated by the command above, so the rename pass could not regenerate it. It
had drifted in four dimensions -- table names, column names, index names and
constraint names -- and none of it showed, because the test skipped wherever it
ran. The test now builds its databases with `schema.Dump` and runs in CI; see
that package's README.

Versioned migration without a CLI is `dialect/sql/schema/versioned.go` --
`Migrations` to plan and apply, `Revisions` to record what ran, `Current` and
`Check` to refuse a database the code has moved past (`7714d7267`). That is the
path to extend, and it is the one covered by
`dialect/sql/schema/integration/versioned_test.go`.

Eight examples under `examples/` called the CLI and were removed with it:
`compositetypes`, `domaintypes`, `enumtypes`, `functionalidx`, `rls`,
`triggers`, `viewcomposite`, `viewschema`. Each composed `ent://ent/schema`
with a hand-written `schema.sql` through an `atlas.hcl` and let `atlas schema
apply` build the database -- a workflow that cannot run here. They were also
red on any desk without the CLI, calling `log.Fatal` where a skip belonged,
and green in CI only because a `CI` guard sat above it.

Six of them declared no ent feature at all: the composite type, domain, enum,
functional index, RLS policy or trigger came from the SQL file, and the ent
schema was ordinary fields. The two that did -- `viewschema` and
`viewcomposite`, which use `ent.View` -- are the reason
[#2](https://github.com/protobuf-orm/ent/issues/2) is open. Nothing was lost
in removing them, because neither had run in a long time.

## Verifying

Run all of it. The engines are not optional -- SQLite alone has hidden real
bugs here before (`d292c1129`).

There are four modules. `cmd` is part of the root one; CI only runs it from its
own directory.

```sh
go test ./...                                            # root module
(cd examples && go test ./...)                           # module
(cd dialect/sql/schema/integration && go test ./...)     # module, SQLite only
(cd entc/integration && go test -race -count=2 ./...)     # module, needs the six services below
```

`compose.test.yaml` holds the same six services the `integration` job declares
-- one old and one new of each engine -- on the ports the tests look for:

```sh
docker compose -f compose.test.yaml up -d --wait
(cd entc/integration && go test -race -count=2 ./...)
docker compose -f compose.test.yaml down
```

| Engine | Version | Port |
|---|---|---|
| MySQL | 8.0 | 3308 |
| MySQL | 8.4 | 3309 |
| MariaDB | 10.11 | 4309 |
| MariaDB | 11.4 | 4310 |
| PostgreSQL | 14 | 5434 |
| PostgreSQL | 17 | 5437 |

The ports are not configurable -- the tests hardcode `localhost:<port>`, so a
Docker daemon that publishes somewhere other than this host needs the ports
forwarded before any of this connects. SQLite runs in-process and needs nothing.

### The two CI gates that catch a bad merge

**Generate.** CI runs `go generate ./... && go mod tidy` in the three modules
and fails if the tree is then dirty. Reproduce it exactly:

```sh
go generate ./... && go mod tidy \
  && (cd examples && go generate ./... && go mod tidy) \
  && (cd entc/integration && go generate ./... && go mod tidy) \
  && git status --porcelain   # must print nothing
```

Note the order: `go generate` runs `go run -mod=mod .../cmd/ent`, which *adds*
entries to `entc/integration/go.sum` that `go mod tidy` then removes. Committing
the intermediate state fails this gate. Commit the tidied files.

**Lint.** `gopls check` over every tracked Go file, through `.github/lint.sh`,
which fails on anything not written down in `.github/lint-allow.txt`. Run it
the same way locally:

```sh
go install golang.org/x/tools/gopls@v0.23.0
.github/lint.sh $(git ls-files '*.go')
```

gopls rather than a linter of its own, so that what is green in an editor is
green in CI. Upstream uses `golangci-lint` with a `.golangci.yml`; take neither.

Two reasons, and the first is the one that decides it. golangci-lint has to be
built with a Go at least as new as the code it reads, and it trails Go by
months: `golangci-lint-action@v5` installs v1.64.8, built with go1.24, and this
module targets 1.27 --

```
can't load config: the Go language version (go1.24) used to build
golangci-lint is lower than the targeted Go version (1.27)
```

-- so the job failed before reading a line of Go, and would again at every Go
release until a golangci-lint caught up. gopls is built from x/tools and has no
such gap. The second reason is smaller: of the fifty-one findings golangci-lint's
configured set produced over gopls's four, one was worth acting on.

## Where a new engine-level test goes

- `dialect/sql/schema/integration/` -- SQLite only, no services. Runs in the
  `unit` job.
- `entc/integration/<topic>/` -- the engine matrix. Anything added here is
  picked up by the `integration` job with no workflow change. Small
  driver-level packages with no codegen are fine: see `enttime` and `entvar`.
- Give a package that creates tables its **own database** on MySql and
  Postgres. Packages in `entc/integration` run in parallel against one server,
  and `-count=2` runs each twice.
