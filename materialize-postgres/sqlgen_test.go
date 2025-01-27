package main

import (
	"database/sql"
	"testing"

	"github.com/estuary/connectors/testsupport"
	"github.com/estuary/protocols/catalog"
	pf "github.com/estuary/protocols/flow"
	sqlDriver "github.com/estuary/protocols/materialize/sql"
	"github.com/stretchr/testify/require"
)

func TestSQLGeneration(t *testing.T) {
	var spec *pf.MaterializationSpec
	require.NoError(t, testsupport.CatalogExtract(t, "testdata/flow.yaml",
		func(db *sql.DB) error {
			var err error
			spec, err = catalog.LoadMaterialization(db, "test/sqlite")
			return err
		}))

	var gen = sqlDriver.PostgresSQLGenerator()
	var table = sqlDriver.TableForMaterialization("test_table", "", gen.IdentifierRenderer, spec.Bindings[0])

	keyCreate, keyInsert, keyJoin, err := buildSQL(&gen, 123, table, spec.Bindings[0].FieldSelection)
	require.NoError(t, err)

	require.Equal(t, `
		CREATE TEMPORARY TABLE flow_load_key_tmp_123 (
			key1 BIGINT NOT NULL, key2 BOOLEAN NOT NULL
		) ON COMMIT DELETE ROWS
		;`, keyCreate)

	require.Equal(t, `
		INSERT INTO flow_load_key_tmp_123 (
			key1, key2
		) VALUES (
			$1, $2
		);`, keyInsert)

	// Note the intentional missing semicolon, as this is a subquery.
	require.Equal(t, `
		SELECT 123, l.flow_document
			FROM test_table AS l
			JOIN flow_load_key_tmp_123 AS r
			ON l.key1 = r.key1 AND l.key2 = r.key2
		`, keyJoin)
}
