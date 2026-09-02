// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package field_test

import (
	"testing"
	"uuid"

	"github.com/protobuf-orm/ent/schema/field"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUuidTextValueScanner covers a Uuid field given an explicit text codec,
// which is what a type of one's own that only marshals to text needs. The
// standard library's UUID does not need it -- see the test below -- but the
// codec is what makes such a type usable at all.
func TestUuidTextValueScanner(t *testing.T) {
	fd := field.Uuid("id", uuid.UUID{}).
		Default(uuid.New).
		ValueScanner(field.TextValueScannerOf[uuid.UUID]()).
		Descriptor()
	require.NoError(t, fd.Err)
	assert.Equal(t, field.TypeUuid, fd.Info.Type)
	assert.Equal(t, "uuid.UUID", fd.Info.Ident)

	vs, ok := fd.ValueScanner.(field.TypeValueScanner[uuid.UUID])
	require.True(t, ok)
	u := uuid.MustParse("cb2f4b3a-1f2e-4f0e-9a54-3f2c9a1e77aa")
	v, err := vs.Value(u)
	require.NoError(t, err)
	assert.Equal(t, []byte(u.String()), v)

	sv := vs.ScanValue()
	require.NoError(t, sv.Scan(u.String()))
	back, err := vs.FromValue(sv)
	require.NoError(t, err)
	assert.Equal(t, u, back)

	// A NULL column scans into the zero value.
	sv = vs.ScanValue()
	require.NoError(t, sv.Scan(nil))
	back, err = vs.FromValue(sv)
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil(), back)
}

// TestUuidWithoutValueScanner covers the same type given with no codec at
// all, which is how a schema ordinarily writes it: database/sql reads and
// writes a uuid.UUID itself, so there is nothing to say.
func TestUuidWithoutValueScanner(t *testing.T) {
	fd := field.Uuid("id").Descriptor()
	require.NoError(t, fd.Err)
	require.Nil(t, fd.ValueScanner, "the standard library's UUID needs no codec")
	require.True(t, fd.Info.RType.DriverType())

	// A type of one's own is not on that list, however much it looks like the
	// one that is: sixteen bytes and a MarshalText are exactly what a
	// uuid.UUID is, and database/sql refuses it all the same.
	type opaque [16]byte
	fd = field.Uuid("id", opaque{}).Descriptor()
	require.EqualError(t, fd.Err,
		`GoType must be a "field.ValueScanner" type, ValueScanner or provide an external ValueScanner`)
}
