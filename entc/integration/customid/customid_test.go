// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package customid

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"testing"
	"uuid"

	"github.com/protobuf-orm/ent/dialect"
	entsql "github.com/protobuf-orm/ent/dialect/sql"
	"github.com/protobuf-orm/ent/dialect/sql/schema"
	"github.com/protobuf-orm/ent/entc/integration/customid/ent"
	"github.com/protobuf-orm/ent/entc/integration/customid/ent/blob"
	"github.com/protobuf-orm/ent/entc/integration/customid/ent/doc"
	"github.com/protobuf-orm/ent/entc/integration/customid/ent/intsid"
	"github.com/protobuf-orm/ent/entc/integration/customid/ent/pet"
	entschema "github.com/protobuf-orm/ent/entc/integration/customid/ent/schema"
	"github.com/protobuf-orm/ent/entc/integration/customid/ent/token"
	"github.com/protobuf-orm/ent/entc/integration/customid/ent/user"
	"github.com/protobuf-orm/ent/entc/integration/customid/ent/valuescan"
	"github.com/protobuf-orm/ent/entc/integration/customid/sid"
	"github.com/protobuf-orm/ent/schema/field"

	atlas "ariga.io/atlas/sql/schema"
	"github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/ncruces/go-sqlite3/driver"
	"github.com/stretchr/testify/require"
)

func TestMySql(t *testing.T) {
	for version, port := range map[string]int{"8": 3308, "84": 3309} {
		addr := net.JoinHostPort("localhost", strconv.Itoa(port))
		t.Run(version, func(t *testing.T) {
			cfg := mysql.Config{
				User: "root", Passwd: "pass", Net: "tcp", Addr: addr,
				AllowNativePasswords: true, ParseTime: true,
			}
			db, err := sql.Open("mysql", cfg.FormatDSN())
			require.NoError(t, err)
			defer db.Close()
			_, err = db.Exec("CREATE DATABASE IF NOT EXISTS custom_id")
			require.NoError(t, err, "creating database")
			defer db.Exec("DROP DATABASE IF EXISTS custom_id")

			cfg.DBName = "custom_id"
			client, err := ent.Open("mysql", cfg.FormatDSN())
			require.NoError(t, err, "connecting to custom_id database")
			err = client.Schema.Create(context.Background(), schema.WithHooks(clearDefault, skipBytesId))
			require.NoError(t, err)
			CustomId(t, client)
		})
	}
}

func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{"14": 5434, "17": 5437} {
		t.Run(version, func(t *testing.T) {
			dsn := fmt.Sprintf("host=localhost port=%d user=postgres password=pass sslmode=disable dbname=test", port)
			db, err := sql.Open(dialect.Postgres, dsn)
			require.NoError(t, err)
			defer db.Close()
			_, err = db.Exec("CREATE SCHEMA IF NOT EXISTS custom_id")
			require.NoError(t, err, "creating schema")
			_, err = db.Exec("SET search_path TO custom_id")
			require.NoError(t, err, "setting schema")
			_, err = db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp" SCHEMA custom_id`)
			require.NoError(t, err, "creating extension")
			defer db.Exec(`DROP EXTENSION "uuid-ossp"`)
			defer db.Exec("DROP SCHEMA custom_id CASCADE")

			client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			err = client.Schema.Create(context.Background(), schema.WithDiffHook(expectOnePetsIndex))
			require.NoError(t, err)
			CustomId(t, client)
			BytesId(t, client)
		})
	}
}

func TestSQLite(t *testing.T) {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	require.NoError(t, client.Schema.Create(context.Background(), schema.WithHooks(clearDefault)))
	CustomId(t, client)
	BytesId(t, client)
}

func CustomId(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	nat := client.User.Create().SaveX(ctx)
	require.Equal(t, 1, nat.Id)
	_, err := client.User.Create().SetId(1).Save(ctx)
	require.True(t, ent.IsConstraintError(err), "duplicate id")
	a8m := client.User.Create().SetId(5).SaveX(ctx)
	require.Equal(t, 5, a8m.Id)

	hub := client.Group.Create().SetId(3).AddUsers(a8m, nat).SaveX(ctx)
	require.Equal(t, 3, hub.Id)
	require.Equal(t, []int{1, 5}, hub.QueryUsers().Order(ent.Asc(user.FieldId)).IdsX(ctx))

	blb := client.Blob.Create().SaveX(ctx)
	require.NotEmpty(t, blb.Id, "use default value")
	id := uuid.New()
	chd := client.Blob.Create().SetId(id).SetParent(blb).SaveX(ctx)
	require.Equal(t, id, chd.Id, "use provided id")
	require.Equal(t, blb.Id, chd.QueryParent().OnlyX(ctx).Id)
	lnk := client.Blob.Create().SetId(uuid.New()).AddLinks(chd, blb).SaveX(ctx)
	require.Equal(t, 2, lnk.QueryLinks().CountX(ctx))
	require.Equal(t, lnk.Id, chd.QueryLinks().OnlyX(ctx).Id)
	require.Equal(t, lnk.Id, blb.QueryLinks().OnlyX(ctx).Id)
	require.Len(t, client.Blob.Query().IdsX(ctx), 3)
	links := lnk.QueryBlobLinks().AllX(ctx)
	require.Len(t, links, 2)
	require.Equal(t, lnk.Id, links[0].BlobId)
	require.NotEqual(t, uuid.Nil(), links[0].LinksId)
	require.Equal(t, lnk.Id, links[1].BlobId)
	require.NotEqual(t, uuid.Nil(), links[1].LinksId)

	pedro := client.Pet.Create().SetId("pedro").SetOwner(a8m).SaveX(ctx)
	require.Equal(t, a8m.Id, pedro.QueryOwner().OnlyIdX(ctx))
	require.Equal(t, pedro.Id, a8m.QueryPets().OnlyIdX(ctx))
	xabi := client.Pet.Create().SetId("xabi").AddFriends(pedro).SetBestFriend(pedro).SaveX(ctx)
	require.Equal(t, "xabi", xabi.Id)
	pedro = client.Pet.Query().Where(pet.HasOwnerWith(user.Id(a8m.Id))).OnlyX(ctx)
	require.Equal(t, "pedro", pedro.Id)

	pets := client.Pet.Query().WithFriends().WithBestFriend().Order(ent.Asc(pet.FieldId)).AllX(ctx)
	require.Len(t, pets, 2)

	require.Equal(t, pedro.Id, pets[0].Id)
	require.NotNil(t, pets[0].Edges.BestFriend)
	require.Equal(t, xabi.Id, pets[0].Edges.BestFriend.Id)
	require.Len(t, pets[0].Edges.Friends, 1)
	require.Equal(t, xabi.Id, pets[0].Edges.Friends[0].Id)

	require.Equal(t, xabi.Id, pets[1].Id)
	require.NotNil(t, pets[1].Edges.BestFriend)
	require.Equal(t, pedro.Id, pets[1].Edges.BestFriend.Id)
	require.Len(t, pets[1].Edges.Friends, 1)
	require.Equal(t, pedro.Id, pets[1].Edges.Friends[0].Id)

	bee := client.Car.Create().SetModel("Chevrolet Camaro").SetOwner(pedro).SaveX(ctx)
	require.NotNil(t, bee)
	bee = client.Car.Query().WithOwner().OnlyX(ctx)
	require.Equal(t, "Chevrolet Camaro", bee.Model)
	require.NotNil(t, bee.Edges.Owner)
	require.Equal(t, pedro.Id, bee.Edges.Owner.Id)

	pets = client.Pet.CreateBulk(
		client.Pet.Create().SetId("luna").SetOwner(a8m).AddFriends(xabi),
		client.Pet.Create().SetId("layla").SetOwner(a8m).AddFriendsIds(pedro.Id),
		client.Pet.Create().AddFriends(pedro, xabi),
	).SaveX(ctx)
	require.Equal(t, "luna", pets[0].Id)
	require.Equal(t, xabi.Id, pets[0].QueryFriends().OnlyIdX(ctx))
	require.Equal(t, "layla", pets[1].Id)
	require.Equal(t, pedro.Id, pets[1].QueryFriends().OnlyIdX(ctx))
	require.Equal(t, []string{"pedro", "xabi"}, pets[2].QueryFriends().Order(ent.Asc(pet.FieldId)).IdsX(ctx))

	u1, u2 := uuid.New(), uuid.New()
	blobs := client.Blob.CreateBulk(
		client.Blob.Create().SetId(u1),
		client.Blob.Create().SetId(u2),
	).SaveX(ctx)
	require.Equal(t, u1, blobs[0].Id)
	require.Equal(t, u2, blobs[1].Id)

	parent := client.Note.Create().SetText("parent").SaveX(ctx)
	require.NotEmpty(t, parent.Id)
	require.NotEmpty(t, parent.Text)
	child := client.Note.Create().SetText("child").SetParent(parent).SaveX(ctx)
	require.NotEmpty(t, child.QueryParent().OnlyIdX(ctx))

	t.Run("ValueScanner Id", func(t *testing.T) {
		id1 := entschema.ValueScanId{V: 10}
		id2 := entschema.ValueScanId{V: 20}
		id3 := entschema.ValueScanId{V: 30}
		id4 := entschema.ValueScanId{V: 40}

		client.ValueScan.Create().SetId(id1).SetName("first").SaveX(ctx)
		client.ValueScan.Create().SetId(id2).SetName("second").SaveX(ctx)
		require.Equal(t, id1, client.ValueScan.GetX(ctx, id1).Id)
		require.Equal(t, id2, client.ValueScan.Query().Where(valuescan.Id(id2)).OnlyX(ctx).Id)
		require.True(t, client.ValueScan.Query().Where(valuescan.Id(id1)).ExistX(ctx))
		require.False(t, client.ValueScan.Query().Where(valuescan.Id(entschema.ValueScanId{V: 999})).ExistX(ctx))

		client.ValueScan.CreateBulk(
			client.ValueScan.Create().SetId(id3).SetName("third"),
			client.ValueScan.Create().SetId(id4).SetName("fourth"),
		).SaveX(ctx)
		require.ElementsMatch(t, []entschema.ValueScanId{id1, id2, id3, id4}, client.ValueScan.Query().IdsX(ctx))

		client.ValueScan.UpdateOneId(id2).SetName("updated").ExecX(ctx)
		require.Equal(t, "updated", client.ValueScan.GetX(ctx, id2).Name)

		var raw []struct {
			Id int
		}
		client.ValueScan.Query().
			Where(valuescan.Name("updated")).
			Select(valuescan.FieldId).
			ScanX(ctx, &raw)
		require.Len(t, raw, 1)
		require.Equal(t, 20, raw[0].Id)
	})

	pdoc := client.Doc.Create().SetText("parent").SaveX(ctx)
	require.NotEmpty(t, pdoc.Id)
	require.NotEmpty(t, pdoc.Text)
	cdoc := client.Doc.Create().SetText("child").SetParent(pdoc).SaveX(ctx)
	require.NotEmpty(t, cdoc.QueryParent().OnlyIdX(ctx))

	t.Run("IntSId", func(t *testing.T) {
		root := client.IntSId.Create().SaveX(ctx)
		require.EqualValues(t, sid.Id("1"), root.Id)
		children := client.IntSId.CreateBulk(
			client.IntSId.Create().SetParent(root),
			client.IntSId.Create().SetParent(root),
		).SaveX(ctx)
		require.EqualValues(t, sid.Id("2"), children[0].Id)
		require.EqualValues(t, sid.Id("3"), children[1].Id)
		el := client.IntSId.Query().Where(intsid.Id(root.Id)).WithChildren().AllX(ctx)
		require.EqualValues(t, 1, len(el))
		require.EqualValues(t, 2, len(el[0].Edges.Children))
		cid := sid.Id("100")
		child := client.IntSId.Create().SetId(cid).SetParent(root).SaveX(ctx)
		require.EqualValues(t, cid, child.Id)
		require.EqualValues(t, root.Id, child.QueryParent().OnlyX(ctx).Id)
	})

	t.Run("Upsert", func(t *testing.T) {
		id := uuid.New()
		client.Blob.Create().
			SetId(id).
			OnConflictColumns(blob.FieldId).
			UpdateNewValues().
			ExecX(ctx)
		require.Zero(t, client.Blob.GetX(ctx, id).Count)
		client.Blob.Create().
			SetId(id).
			OnConflictColumns(blob.FieldId).
			Update(func(set *ent.BlobUpsert) {
				set.AddCount(1)
			}).
			ExecX(ctx)
		require.Equal(t, 1, client.Blob.GetX(ctx, id).Count)

		d := client.Doc.Create().SaveX(ctx)
		client.Doc.Create().
			SetId(d.Id).
			OnConflictColumns(doc.FieldId).
			SetText("Hello World").
			UpdateNewValues().
			ExecX(ctx)
		require.Equal(t, "Hello World", client.Doc.GetX(ctx, d.Id).Text)
	})

	t.Run("Other Id", func(t *testing.T) {
		o := client.Other.Create().SaveX(ctx)
		require.NotEmpty(t, o.Id.String())

		o = client.Other.Create().SetId(sid.NewLength(15)).SaveX(ctx)
		require.NotEmpty(t, o.Id.String())
	})

	t.Run("CustomId edge", func(t *testing.T) {
		a := client.Account.Create().SetEmail("test@example.org").SaveX(ctx)
		require.NotEmpty(t, a.Id)

		tk := client.Token.Create().SetAccountId(a.Id).SetBody("token").SaveX(ctx)
		require.NotEmpty(t, tk.Id)

		ta := client.Token.Query().Where(token.Body("token")).WithAccount().FirstX(ctx)
		require.Equal(t, tk.Id, ta.Id)
		require.NotNil(t, ta.Edges.Account)
		require.Equal(t, a.Id, ta.Edges.Account.Id)
	})

	t.Run("Uuid compatible", func(t *testing.T) {
		l := client.Link.Create().SaveX(ctx)
		require.NotEmpty(t, l.Id)
		require.Len(t, l.LinkInformation, 1)
		require.Equal(t, "ent", l.LinkInformation["ent"].Name)
		require.Equal(t, "https://entgo.io/", l.LinkInformation["ent"].Link)
	})
}

func BytesId(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	s := client.Session.Create().SaveX(ctx)
	require.NotEmpty(t, s.Id)
	client.Device.Create().SetActiveSession(s).AddSessionsIds(s.Id).SaveX(ctx)
	d := client.Device.Query().WithActiveSession().WithSessions().OnlyX(ctx)
	require.Equal(t, s.Id, d.Edges.ActiveSession.Id)
	require.Equal(t, s.Id, d.Edges.Sessions[0].Id)
}

// clearDefault clears the id's default for non-postgres dialects.
func clearDefault(c schema.Creator) schema.Creator {
	return schema.CreateFunc(func(ctx context.Context, tables ...*schema.Table) error {
		// Drop DEFAULT clause for MySql without changing the tables.
		ct := make([]*schema.Table, len(tables))
		copy(ct, tables)
		*ct[1] = *tables[1]
		ct[1].Columns = append([]*schema.Column(nil), tables[1].Columns...)
		*ct[1].Columns[0] = *tables[1].Columns[0]
		ct[1].Columns[0].Default = nil
		return c.Create(ctx, ct...)
	})
}

// skipBytesId tables with blob ids from the migration.
func skipBytesId(c schema.Creator) schema.Creator {
	return schema.CreateFunc(func(ctx context.Context, tables ...*schema.Table) error {
		t := make([]*schema.Table, 0, len(tables))
		for i := range tables {
			if tables[i].PrimaryKey[0].Type == field.TypeBytes {
				continue
			}
			t = append(t, tables[i])
		}
		return c.Create(ctx, t...)
	})
}

// expectOnePetsIndex expects that pets table contains only one index.
func expectOnePetsIndex(next schema.Differ) schema.Differ {
	return schema.DiffFunc(func(current, desired *atlas.Schema) ([]atlas.Change, error) {
		changes, err := next.Diff(current, desired)
		for _, c := range changes {
			addT, ok := c.(*atlas.AddTable)
			if !ok || addT.T.Name != pet.Table {
				continue
			}
			if n := len(addT.T.Indexes); n != 1 {
				return nil, fmt.Errorf("expect only one index, but got: %d", n)
			}
		}
		return changes, err
	})
}
