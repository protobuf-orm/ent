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

// TestUuidTextValueScanner covers a Uuid type that marshals to text but
// implements neither driver.Valuer nor sql.Scanner, as the one in the
// standard library does.
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

// TestUuidWithoutValueScanner keeps the requirement that a Uuid type without
// an external ValueScanner must implement the database interfaces itself.
func TestUuidWithoutValueScanner(t *testing.T) {
	fd := field.Uuid("id", uuid.UUID{}).Descriptor()
	require.EqualError(t, fd.Err,
		`GoType must be a "field.ValueScanner" type, ValueScanner or provide an external ValueScanner`)
}
