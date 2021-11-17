package sqlcapture

import "context"

// ChangeEvent represents either an Insert/Update/Delete operation on a specific
// row in the database, or a Commit event which indicates that the database is at
// a consistent point from which we could restart in the future.
type ChangeEvent struct {
	Type      string
	Cursor    string // Only set for 'Commit' events
	Namespace string
	Table     string
	Fields    map[string]interface{}
}

// A ChangeEvent can have one of these types:
//   Commit: The database is at a consistent point (between transactions) here.
//   Insert: A new row has been added to a table.
//   Update: An existing row in the table has been modified.
//   Delete: An existing row in the table has been deleted.
const (
	ChangeTypeCommit = "Commit"
	ChangeTypeInsert = "Insert"
	ChangeTypeUpdate = "Update"
	ChangeTypeDelete = "Delete"
)

// Database represents the operations which must be performed on a specific database
// during the course of a capture in order to perform discovery, backfill preexisting
// data, and process replicated change events.
type Database interface {
	// TODO(wgd): Document specific methods
	Connect(ctx context.Context) error
	// TODO(wgd): Document specific methods
	Close(ctx context.Context) error
	// TODO(wgd): Document specific methods
	StartReplication(ctx context.Context, startCursor string) (ReplicationStream, error)
	// TODO(wgd): Document specific methods
	WriteWatermark(ctx context.Context) (string, error)
	// TODO(wgd): Document specific methods
	WatermarksTable() string
	// TODO(wgd): Document specific methods
	ScanTableChunk(ctx context.Context, streamID string, keyColumns []string, resumeKey []interface{}) ([]ChangeEvent, error)
	// TODO(wgd): Document specific methods
	DiscoverTables(ctx context.Context) (map[string]TableInfo, error)
	// TODO(wgd): Document specific methods
	TranslateDBToJSONType(typeName string) (string, error)
	// TODO(wgd): Document specific methods
	TranslateRecordField(val interface{}) (interface{}, error)
}

// ReplicationStream represents the process of receiving change events
// from a database, managing keepalives and status updates, and translating
// these changes into a stream of ChangeEvents.
type ReplicationStream interface {
	Events() <-chan ChangeEvent
	Commit(ctx context.Context, cursor string) error
	Close(ctx context.Context) error
}

// TableInfo holds metadata about a specific table in the database, and
// is used during discovery to automatically generate catalog information.
type TableInfo struct {
	Name       string       // The PostgreSQL table name.
	Schema     string       // The PostgreSQL schema (a namespace, in normal parlance) which contains the table.
	Columns    []ColumnInfo // Information about each column of the table.
	PrimaryKey []string     // An ordered list of the column names which together form the table's primary key.
}

// ColumnInfo holds metadata about a specific column of some table in the
// database, and is used during discovery to automatically generate catalog
// information.
type ColumnInfo struct {
	Name        string // The name of the column.
	Index       int    // The ordinal position of this column in a row.
	TableName   string // The name of the table to which this column belongs.
	TableSchema string // The schema of the table to which this column belongs.
	IsNullable  bool   // True if the column can contain nulls.
	DataType    string // The PostgreSQL type name of this column.
}
