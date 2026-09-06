# Getting Started Example

The example from the getting-started guide.

### Generate assets

```console
go generate ./...
```

### Create the schema

`start.go` calls `client.Schema.Create(ctx)`, which brings an empty database to
the shape the ent schema describes. That is the right thing for a database
nobody has deployed yet.

For one that has, write the change to a file instead and review it like any
other code -- see `examples/migration`.
