package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/estuary/connectors/sqlcapture"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/sirupsen/logrus"
)

// DiscoverTables queries the database to produce a list of all tables
// (with the exception of some internal system schemas) with information
// about their column types and primary key.
func (db *postgresDatabase) DiscoverTables(ctx context.Context) (map[string]sqlcapture.TableInfo, error) {
	// Get lists of all columns and primary keys in the database
	var columns, err = getColumns(ctx, db.conn)
	if err != nil {
		return nil, fmt.Errorf("unable to list database columns: %w", err)
	}
	primaryKeys, err := getPrimaryKeys(ctx, db.conn)
	if err != nil {
		return nil, fmt.Errorf("unable to list database primary keys: %w", err)
	}

	// Aggregate column and primary key information into TableInfo structs
	// using a map from fully-qualified "<schema>.<name>" table names to
	// the corresponding TableInfo.
	var tableMap = make(map[string]sqlcapture.TableInfo)
	for _, column := range columns {
		var id = column.TableSchema + "." + column.TableName
		var info, ok = tableMap[id]
		if !ok {
			info = sqlcapture.TableInfo{Schema: column.TableSchema, Name: column.TableName}
		}
		info.Columns = append(info.Columns, column)
		tableMap[id] = info
	}
	for id, key := range primaryKeys {
		// The `getColumns()` query implements the "exclude system schemas" logic,
		// so here we ignore primary key information for tables we don't care about.
		var info, ok = tableMap[id]
		if !ok {
			continue
		}
		logrus.WithField("table", id).WithField("key", key).Debug("queried primary key")
		info.PrimaryKey = key
		tableMap[id] = info
	}
	return tableMap, nil
}

// TranslateRecordField "translates" a value from the PostgreSQL driver into
// an appropriate JSON-encodable output format. As a concrete example, the
// PostgreSQL `cidr` type becomes a `*net.IPNet`, but the default JSON
// marshalling of a `net.IPNet` isn't a great fit and we'd prefer to use
// the `String()` method to get the usual "192.168.100.0/24" notation.
func (db *postgresDatabase) TranslateRecordField(val interface{}) (interface{}, error) {
	switch x := val.(type) {
	case *net.IPNet:
		return x.String(), nil
	case net.HardwareAddr:
		return x.String(), nil
	case [16]uint8: // UUIDs
		var s = new(strings.Builder)
		for i := range x {
			if i == 4 || i == 6 || i == 8 || i == 10 {
				s.WriteString("-")
			}
			fmt.Fprintf(s, "%02x", x[i])
		}
		return s.String(), nil
	}
	if _, ok := val.(json.Marshaler); ok {
		return val, nil
	}
	if enc, ok := val.(pgtype.TextEncoder); ok {
		var bs, err = enc.EncodeText(nil, nil)
		return string(bs), err
	}
	return val, nil
}

func (db *postgresDatabase) TranslateDBToJSONType(typeName string) (string, error) {
	var jsonType, ok = postgresTypeToJSON[typeName]
	if !ok {
		return "", fmt.Errorf("unknown column type %q", jsonType)
	}
	return jsonType, nil
}

var postgresTypeToJSON = map[string]string{
	"bool": `{"type":"boolean"}`,

	"int2": `{"type":"integer"}`,
	"int4": `{"type":"integer"}`,
	"int8": `{"type":"integer"}`,

	// TODO(wgd): More systematic treatment of arrays?
	"_int2":   `{"type":"string"}`,
	"_int4":   `{"type":"string"}`,
	"_int8":   `{"type":"string"}`,
	"_float4": `{"type":"string"}`,
	"_text":   `{"type":"string"}`,

	"numeric": `{"type":"number"}`,
	"float4":  `{"type":"number"}`,
	"float8":  `{"type":"number"}`,

	"varchar": `{"type":"string"}`,
	"bpchar":  `{"type":"string"}`,
	"text":    `{"type":"string"}`,
	"bytea":   `{"type":"string","contentEncoding":"base64"}`,
	"xml":     `{"type":"string"}`,
	"bit":     `{"type":"string"}`,
	"varbit":  `{"type":"string"}`,

	"json":     `{}`,
	"jsonb":    `{}`,
	"jsonpath": `{"type":"string"}`,

	// Domain-Specific Types
	"date":        `{"type":"string","format":"date-time"}`,
	"timestamp":   `{"type":"string","format":"date-time"}`,
	"timestamptz": `{"type":"string","format":"date-time"}`,
	"time":        `{"type":"integer"}`,
	"timetz":      `{"type":"string","format":"time"}`,
	"interval":    `{"type":"string"}`,
	"money":       `{"type":"string"}`,
	"point":       `{"type":"string"}`,
	"line":        `{"type":"string"}`,
	"lseg":        `{"type":"string"}`,
	"box":         `{"type":"string"}`,
	"path":        `{"type":"string"}`,
	"polygon":     `{"type":"string"}`,
	"circle":      `{"type":"string"}`,
	"inet":        `{"type":"string"}`,
	"cidr":        `{"type":"string"}`,
	"macaddr":     `{"type":"string"}`,
	"macaddr8":    `{"type":"string"}`,
	"tsvector":    `{"type":"string"}`,
	"tsquery":     `{"type":"string"}`,
	"uuid":        `{"type":"string","format":"uuid"}`,
}

const queryDiscoverColumns = `
  SELECT table_schema, table_name, ordinal_position, column_name, is_nullable::boolean, udt_name
  FROM information_schema.columns
  WHERE table_schema != 'pg_catalog' AND table_schema != 'information_schema'
        AND table_schema != 'pg_internal' AND table_schema != 'catalog_history'
  ORDER BY table_schema, table_name, ordinal_position;`

func getColumns(ctx context.Context, conn *pgx.Conn) ([]sqlcapture.ColumnInfo, error) {
	var columns []sqlcapture.ColumnInfo
	var sc sqlcapture.ColumnInfo
	var _, err = conn.QueryFunc(ctx, queryDiscoverColumns, nil,
		[]interface{}{&sc.TableSchema, &sc.TableName, &sc.Index, &sc.Name, &sc.IsNullable, &sc.DataType},
		func(r pgx.QueryFuncRow) error {
			columns = append(columns, sc)
			return nil
		})
	return columns, err
}

// Query copied from pgjdbc's method PgDatabaseMetaData.getPrimaryKeys() with
// the always-NULL `TABLE_CAT` column omitted.
//
// See: https://github.com/pgjdbc/pgjdbc/blob/master/pgjdbc/src/main/java/org/postgresql/jdbc/PgDatabaseMetaData.java#L2134
const queryDiscoverPrimaryKeys = `
  SELECT result.TABLE_SCHEM, result.TABLE_NAME, result.COLUMN_NAME, result.KEY_SEQ
  FROM (
    SELECT n.nspname AS TABLE_SCHEM,
      ct.relname AS TABLE_NAME, a.attname AS COLUMN_NAME,
      (information_schema._pg_expandarray(i.indkey)).n AS KEY_SEQ, ci.relname AS PK_NAME,
      information_schema._pg_expandarray(i.indkey) AS KEYS, a.attnum AS A_ATTNUM
    FROM pg_catalog.pg_class ct
      JOIN pg_catalog.pg_attribute a ON (ct.oid = a.attrelid)
      JOIN pg_catalog.pg_namespace n ON (ct.relnamespace = n.oid)
      JOIN pg_catalog.pg_index i ON (a.attrelid = i.indrelid)
      JOIN pg_catalog.pg_class ci ON (ci.oid = i.indexrelid)
    WHERE i.indisprimary
  ) result
  WHERE result.A_ATTNUM = (result.KEYS).x
  ORDER BY result.table_name, result.pk_name, result.key_seq;
`

// getPrimaryKeys queries the database to produce a map from table names to
// primary keys. Table names are fully qualified as "<schema>.<name>", and
// primary keys are represented as a list of column names, in the order that
// they form the table's primary key.
func getPrimaryKeys(ctx context.Context, conn *pgx.Conn) (map[string][]string, error) {
	var keys = make(map[string][]string)
	var tableSchema, tableName, columnName string
	var columnIndex int
	var _, err = conn.QueryFunc(ctx, queryDiscoverPrimaryKeys, nil,
		[]interface{}{&tableSchema, &tableName, &columnName, &columnIndex},
		func(r pgx.QueryFuncRow) error {
			var id = fmt.Sprintf("%s.%s", tableSchema, tableName)
			keys[id] = append(keys[id], columnName)
			if columnIndex != len(keys[id]) {
				return fmt.Errorf("primary key column %q appears out of order (expected index %d, in context %q)", columnName, columnIndex, keys[id])
			}
			return nil
		})
	return keys, err
}
