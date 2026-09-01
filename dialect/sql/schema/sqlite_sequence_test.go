// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package schema

import (
	"testing"

	"ariga.io/atlas/sql/migrate"

	"github.com/stretchr/testify/require"
)

func TestFixSequenceQuoting(t *testing.T) {
	plan := &migrate.Plan{Changes: []*migrate.Change{
		{
			Cmd:     `INSERT INTO sqlite_sequence (name, seq) VALUES ("users", 4294967296)`,
			Reverse: `UPDATE sqlite_sequence SET seq = 0 WHERE name = "users"`,
		},
		{
			// A name that needs escaping on the way into a string literal.
			Cmd: `INSERT INTO sqlite_sequence (name, seq) VALUES ("o'neill", 1)`,
		},
		{
			// Reverse may hold several statements.
			Cmd:     `INSERT INTO sqlite_sequence (name, seq) VALUES ("pets", 2)`,
			Reverse: []string{`UPDATE sqlite_sequence SET seq = 0 WHERE name = "pets"`, `SELECT 1`},
		},
		{
			// Unrelated statements keep their quoting, including identifiers.
			Cmd:     "CREATE INDEX `user_phone` ON `user` (`phone`) WHERE \"phone\" <> ''",
			Reverse: "DROP INDEX `user_phone`",
		},
	}}
	fixSequenceQuoting(plan)
	require.Equal(t, `INSERT INTO sqlite_sequence (name, seq) VALUES ('users', 4294967296)`, plan.Changes[0].Cmd)
	require.Equal(t, `UPDATE sqlite_sequence SET seq = 0 WHERE name = 'users'`, plan.Changes[0].Reverse)
	require.Equal(t, `INSERT INTO sqlite_sequence (name, seq) VALUES ('o''neill', 1)`, plan.Changes[1].Cmd)
	require.Equal(t, `INSERT INTO sqlite_sequence (name, seq) VALUES ('pets', 2)`, plan.Changes[2].Cmd)
	require.Equal(t, []string{`UPDATE sqlite_sequence SET seq = 0 WHERE name = 'pets'`, `SELECT 1`}, plan.Changes[2].Reverse)
	require.Equal(t, "CREATE INDEX `user_phone` ON `user` (`phone`) WHERE \"phone\" <> ''", plan.Changes[3].Cmd)
	require.Equal(t, "DROP INDEX `user_phone`", plan.Changes[3].Reverse)
}
