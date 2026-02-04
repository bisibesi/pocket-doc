package extractor

import (
	"context"
	"dbms-to-document/internal/model"
)

// Extractor defines the interface for extracting database metadata
// All implementations must support granular extraction of each object type
type Extractor interface {
	// Connect establishes connection to the database
	Connect(ctx context.Context) error

	// Close releases database resources
	Close() error

	// GetDatabaseInfo retrieves basic database information
	GetDatabaseInfo(ctx context.Context) (name, version string, err error)

	// GetTables extracts all table metadata (without source code)
	GetTables(ctx context.Context) ([]model.Table, error)

	// GetViews extracts all view metadata (without source code)
	GetViews(ctx context.Context) ([]model.View, error)

	// GetRoutines extracts all stored procedures and functions (signature only, no body)
	GetRoutines(ctx context.Context) ([]model.Routine, error)

	// GetIndexes extracts all index metadata
	GetIndexes(ctx context.Context) ([]model.Index, error)

	// GetSequences extracts all sequence metadata
	GetSequences(ctx context.Context) ([]model.Sequence, error)

	// GetTriggers extracts all trigger metadata (no trigger body/source)
	GetTriggers(ctx context.Context) ([]model.Trigger, error)

	// GetSynonyms extracts all synonym metadata
	GetSynonyms(ctx context.Context) ([]model.Synonym, error)

	// ExtractSchema performs a complete extraction of all database objects
	ExtractSchema(ctx context.Context) (*model.Schema, error)
}
