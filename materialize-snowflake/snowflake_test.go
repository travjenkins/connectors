package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/bradleyjkemp/cupaloy"
	"github.com/estuary/connectors/testsupport"
	"github.com/estuary/protocols/catalog"
	pf "github.com/estuary/protocols/flow"
	pm "github.com/estuary/protocols/materialize"
	sqlDriver "github.com/estuary/protocols/materialize/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestQueryGeneration(t *testing.T) {
	var spec *pf.MaterializationSpec
	require.NoError(t, testsupport.CatalogExtract(t, "testdata/flow.yaml",
		func(db *sql.DB) error {
			var err error
			spec, err = catalog.LoadMaterialization(db, "test/sqlite")
			return err
		}))

	var gen = SQLGenerator()
	var table = sqlDriver.TableForMaterialization("test_table", "", gen.IdentifierRenderer, spec.Bindings[0])

	var loadUUID = uuid.UUID{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	var storeUUID = uuid.UUID{15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	var keyJoinSQL, copyIntoSQL, mergeIntoSQL = BuildSQL(123, table, spec.Bindings[0].FieldSelection, loadUUID, storeUUID)

	// Note the intentional missing semicolon, as this is a subquery.
	require.Equal(t, `
		SELECT 123, test_table.flow_document
		FROM test_table
		JOIN (
			SELECT $1[0] AS key1, $1[1] AS key2
			FROM @flow_v1/00010203-0405-0607-0809-0a0b0c0d0e0f
		) AS r
		ON test_table.key1 = r.key1 AND test_table.key2 = r.key2
		`,
		keyJoinSQL)

	require.Equal(t, `
		COPY INTO test_table (
			key1, key2, boolean, integer, number, string, flow_document
		) FROM (
			SELECT $1[0] AS key1, $1[1] AS key2, $1[2] AS boolean, $1[3] AS integer, $1[4] AS number, $1[5] AS string, $1[6] AS flow_document
			FROM @flow_v1/0f0e0d0c-0b0a-0908-0706-050403020100
		)
		;`,
		copyIntoSQL)

	require.Equal(t, `
		MERGE INTO test_table
		USING (
			SELECT $1[0] AS key1, $1[1] AS key2, $1[2] AS boolean, $1[3] AS integer, $1[4] AS number, $1[5] AS string, $1[6] AS flow_document
			FROM @flow_v1/0f0e0d0c-0b0a-0908-0706-050403020100
		) AS r
		ON test_table.key1 = r.key1 AND test_table.key2 = r.key2
		WHEN MATCHED AND IS_NULL_VALUE(r.flow_document) THEN
			DELETE
		WHEN MATCHED THEN
			UPDATE SET test_table.boolean = r.boolean, test_table.integer = r.integer, test_table.number = r.number, test_table.string = r.string, test_table.flow_document = r.flow_document
		WHEN NOT MATCHED THEN
			INSERT (key1, key2, boolean, integer, number, string, flow_document)
			VALUES (r.key1, r.key2, r.boolean, r.integer, r.number, r.string, r.flow_document)
		;`,
		mergeIntoSQL)
}

func TestSpecification(t *testing.T) {
	var resp, err = newSnowflakeDriver().
		Spec(context.Background(), &pm.SpecRequest{EndpointType: pf.EndpointType_AIRBYTE_SOURCE})
	require.NoError(t, err)

	formatted, err := json.MarshalIndent(resp, "", "  ")
	require.NoError(t, err)

	cupaloy.SnapshotT(t, formatted)
}
