package exporter

import (
	"pocket-doc/internal/model"
	"io"
)

// Exporter defines the interface for exporting schema to various formats
type Exporter interface {
	// Export writes the schema to the provided writer in the specific format
	Export(schema *model.Schema, w io.Writer) error

	// Format returns the format name (e.g., "xlsx", "docx", "html", "pdf")
	Format() string

	// MimeType returns the MIME type for HTTP response headers
	MimeType() string

	// FileExtension returns the file extension (e.g., ".xlsx", ".docx")
	FileExtension() string
}

// Config holds common configuration for all exporters
type Config struct {
	// Language for templates (en, ko)
	Language string

	// IncludeTOC enables Table of Contents generation
	IncludeTOC bool

	// IncludeCoverPage adds a cover page (for Word/PDF)
	IncludeCoverPage bool

	// CompanyName for cover page
	CompanyName string

	// ProjectName for cover page
	ProjectName string

	// Author name
	Author string

	// ExcludeTypes allows skipping certain object types
	ExcludeTypes []string

	// ColorScheme for Excel/Word styling ("default", "professional", "minimal")
	ColorScheme string
}
