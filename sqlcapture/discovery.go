package sqlcapture

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/estuary/protocols/airbyte"
	"github.com/sirupsen/logrus"
)

// DiscoverCatalog queries the database and generates an Airbyte Catalog
// describing the available tables and their columns.
func DiscoverCatalog(ctx context.Context, db Database) (*airbyte.Catalog, error) {
	if err := db.Connect(ctx); err != nil {
		return nil, err
	}
	defer db.Close(ctx)

	tables, err := db.DiscoverTables(ctx)
	if err != nil {
		return nil, err
	}

	var catalog = new(airbyte.Catalog)
	for _, table := range tables {
		logrus.WithFields(logrus.Fields{
			"table":      table.Name,
			"namespace":  table.Schema,
			"primaryKey": table.PrimaryKey,
		}).Debug("discovered table")

		var fields = make(map[string]json.RawMessage)
		for _, column := range table.Columns {
			var jsonType, err = db.TranslateDBToJSONType(column.DataType)
			if err != nil {
				return nil, fmt.Errorf("error translating column type to JSON schema: %w", err)
			}
			if column.IsNullable && jsonType != "{}" {
				jsonType = fmt.Sprintf(`{"anyOf":[%s,{"type":"null"}]}`, jsonType)
			}
			fields[column.Name] = json.RawMessage(jsonType)
		}
		var schema, err = json.Marshal(map[string]interface{}{
			"type":       "object",
			"required":   table.PrimaryKey,
			"properties": fields,
		})
		if err != nil {
			return nil, fmt.Errorf("error marshalling schema JSON: %w", err)
		}

		logrus.WithFields(logrus.Fields{
			"table":     table.Name,
			"namespace": table.Schema,
			"columns":   table.Columns,
			"schema":    string(schema),
		}).Debug("translated table schema")

		var sourceDefinedPrimaryKey [][]string
		for _, colName := range table.PrimaryKey {
			sourceDefinedPrimaryKey = append(sourceDefinedPrimaryKey, []string{colName})
		}

		catalog.Streams = append(catalog.Streams, airbyte.Stream{
			Name:                    table.Name,
			Namespace:               table.Schema,
			JSONSchema:              json.RawMessage(schema),
			SupportedSyncModes:      airbyte.AllSyncModes,
			SourceDefinedCursor:     true,
			SourceDefinedPrimaryKey: sourceDefinedPrimaryKey,
		})
	}
	return catalog, err
}
