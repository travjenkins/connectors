package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/estuary/connectors/sqlcapture"
	"github.com/estuary/protocols/airbyte"
	"github.com/jackc/pgx/v4"
	"github.com/sirupsen/logrus"
)

func main() {
	sqlcapture.AirbyteMain(spec, func(configFile airbyte.ConfigFile) (sqlcapture.Database, error) {
		var config Config
		if err := configFile.Parse(&config); err != nil {
			return nil, fmt.Errorf("error parsing config file: %w", err)
		}
		return &postgresDatabase{config: &config}, nil
	})
}

// Config tells the connector how to connect to the source database and can
// optionally be used to customize some other parameters such as polling timeout.
type Config struct {
	ConnectionURI   string `json:"connectionURI"`
	SlotName        string `json:"slot_name"`
	PublicationName string `json:"publication_name"`
	WatermarksTable string `json:"watermarks_table"`
}

// Validate checks that the configuration passes some basic sanity checks, and
// fills in default values when optional parameters are unset.
func (c *Config) Validate() error {
	if c.ConnectionURI == "" {
		return fmt.Errorf("Database Connection URI must be set")
	}
	if c.SlotName == "" {
		c.SlotName = "flow_slot"
	}
	if c.PublicationName == "" {
		c.PublicationName = "flow_publication"
	}
	if c.WatermarksTable == "" {
		c.WatermarksTable = "public.flow_watermarks"
	}
	return nil
}

var spec = airbyte.Spec{
	SupportsIncremental:     true,
	ConnectionSpecification: json.RawMessage(configSchema),
}

const configSchema = `{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"title":   "Postgres Source Spec",
	"type":    "object",
	"properties": {
		"connectionURI": {
			"type":        "string",
			"title":       "Database Connection URI",
			"description": "Connection parameters, as a libpq-compatible connection string",
			"default":     "postgres://flow:flow@localhost:5432/flow"
		},
		"slot_name": {
			"type":        "string",
			"title":       "Replication Slot Name",
			"description": "The name of the PostgreSQL replication slot to replicate from",
			"default":     "flow_slot"
		},
		"publication_name": {
			"type":        "string",
			"title":       "Publication Name",
			"description": "The name of the PostgreSQL publication to replicate from",
			"default":     "flow_publication"
		},
		"watermarks_table": {
			"type":        "string",
			"title":       "Watermarks Table",
			"description": "The name of the table used for watermark writes during backfills",
			"default":     "public.flow_watermarks"
		}
	},
	"required": [ "connectionURI" ]
}`

type postgresDatabase struct {
	config *Config
	conn   *pgx.Conn
}

func (db *postgresDatabase) Connect(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"uri":  db.config.ConnectionURI,
		"slot": db.config.SlotName,
	}).Info("initializing connector")

	// Normal database connection used for table scanning
	var conn, err = pgx.Connect(ctx, db.config.ConnectionURI)
	if err != nil {
		return fmt.Errorf("unable to connect to database: %w", err)
	}
	db.conn = conn
	return nil
}

func (db *postgresDatabase) Close(ctx context.Context) error {
	if err := db.conn.Close(ctx); err != nil {
		return fmt.Errorf("error closing database connection: %w", err)
	}
	return nil
}
