package extractor

import (
	"context"
	"dbms-to-document/internal/extractor/mssql"
	"dbms-to-document/internal/extractor/mysql"
	"dbms-to-document/internal/extractor/oracle"
	"dbms-to-document/internal/extractor/postgres"
	"dbms-to-document/internal/model"
	"fmt"
	"strings"
)

// DBExtractor is the unified interface for all database extractors
type DBExtractor interface {
	Connect(ctx context.Context) error
	Close() error
	GetDatabaseInfo(ctx context.Context) (name, version string, err error)
	GetTables(ctx context.Context) ([]model.Table, error)
	GetViews(ctx context.Context) ([]model.View, error)
	GetRoutines(ctx context.Context) ([]model.Routine, error)
	GetSequences(ctx context.Context) ([]model.Sequence, error)
	GetTriggers(ctx context.Context) ([]model.Trigger, error)
	GetSynonyms(ctx context.Context) ([]model.Synonym, error)
	ExtractSchema(ctx context.Context) (*model.Schema, error)
}

// NewDBExtractor creates a database extractor based on type
func NewDBExtractor(dbType string, config Config) (DBExtractor, error) {
	dbType = strings.ToLower(strings.TrimSpace(dbType))

	switch dbType {
	case "oracle":
		cfg := oracle.Config{
			Host:         config.Host,
			Port:         config.Port,
			ServiceName:  config.Database,
			Username:     config.Username,
			Password:     config.Password,
			SchemaFilter: config.SchemaFilter,
		}
		return oracle.NewExtractor(cfg)

	case "mysql":
		cfg := mysql.Config{
			Host:         config.Host,
			Port:         config.Port,
			Database:     config.Database,
			Username:     config.Username,
			Password:     config.Password,
			SchemaFilter: config.SchemaFilter,
		}
		return mysql.NewExtractor(cfg)

	case "postgresql", "postgres", "pg":
		sslMode := config.SSLMode
		if sslMode == "" {
			sslMode = "disable"
		}
		cfg := postgres.Config{
			Host:         config.Host,
			Port:         config.Port,
			Database:     config.Database,
			Username:     config.Username,
			Password:     config.Password,
			SSLMode:      sslMode,
			SchemaFilter: config.SchemaFilter,
		}
		return postgres.NewExtractor(cfg)

	case "mssql", "sqlserver":
		encrypt := "disable"
		if config.SSLMode == "require" || config.SSLMode == "true" {
			encrypt = "true"
		}
		cfg := mssql.Config{
			Host:         config.Host,
			Port:         config.Port,
			Database:     config.Database,
			Username:     config.Username,
			Password:     config.Password,
			Encrypt:      encrypt,
			SchemaFilter: config.SchemaFilter,
		}
		return mssql.NewExtractor(cfg)

	default:
		return nil, fmt.Errorf("unsupported database type: %s (supported: oracle, mysql, postgresql, mssql)", dbType)
	}
}

// GetSupportedDatabases returns list of supported database types
func GetSupportedDatabases() []string {
	return []string{"oracle", "mysql", "postgresql", "mssql"}
}

// Config holds unified database configuration
type Config struct {
	Host         string
	Port         int
	Database     string
	Username     string
	Password     string
	SSLMode      string
	SchemaFilter []string
}
