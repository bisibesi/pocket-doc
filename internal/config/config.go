package config

// Config represents the complete application configuration
// All fields use mapstructure tags for Viper compatibility
type Config struct {
	Database DatabaseConfig `mapstructure:"database"`
	Output   OutputConfig   `mapstructure:"output"`
	Extract  ExtractConfig  `mapstructure:"extract"`
	Logging  LogConfig      `mapstructure:"logging"`
}

// DatabaseConfig holds database connection settings
type DatabaseConfig struct {
	Type         string            `mapstructure:"type"` // oracle, postgresql, mysql, sqlserver, sqlite
	Host         string            `mapstructure:"host"`
	Port         int               `mapstructure:"port"`
	Database     string            `mapstructure:"database"`
	Username     string            `mapstructure:"username"`
	Password     string            `mapstructure:"password"`
	SSLMode      string            `mapstructure:"ssl_mode"`      // disable, require, verify-ca, verify-full
	Timeout      int               `mapstructure:"timeout"`       // connection timeout in seconds
	SchemaFilter []string          `mapstructure:"schema_filter"` // Filter by schema/owner
	Options      map[string]string `mapstructure:"options"`       // additional driver-specific options
}

// OutputConfig controls document generation settings
type OutputConfig struct {
	Format           string   `mapstructure:"format"` // markdown, html, pdf, xlsx, docx
	OutputDir        string   `mapstructure:"output_dir"`
	FileName         string   `mapstructure:"file_name"`
	IncludeTOC       bool     `mapstructure:"include_toc"`        // Table of Contents
	IncludeCoverPage bool     `mapstructure:"include_cover_page"` // Cover page for Word/PDF
	IncludeERD       bool     `mapstructure:"include_erd"`        // Entity Relationship Diagram
	SplitByType      bool     `mapstructure:"split_by_type"`      // Separate files per object type
	Language         string   `mapstructure:"language"`           // en, ko for templates
	Template         string   `mapstructure:"template"`           // custom template path
	ExcludeTypes     []string `mapstructure:"exclude_types"`      // Object types to skip
	CompanyName      string   `mapstructure:"company_name"`       // For cover page
	ProjectName      string   `mapstructure:"project_name"`       // For cover page
	Author           string   `mapstructure:"author"`             // Document author
	ColorScheme      string   `mapstructure:"color_scheme"`       // default, professional, minimal
}

// ExtractConfig controls what metadata to extract
type ExtractConfig struct {
	IncludeTables    bool `mapstructure:"include_tables"`
	IncludeViews     bool `mapstructure:"include_views"`
	IncludeRoutines  bool `mapstructure:"include_routines"`
	IncludeSequences bool `mapstructure:"include_sequences"`
	IncludeTriggers  bool `mapstructure:"include_triggers"`
	IncludeSynonyms  bool `mapstructure:"include_synonyms"`
	IncludeIndexes   bool `mapstructure:"include_indexes"`

	// Filter options
	SchemaFilter  []string `mapstructure:"schema_filter"`  // Only extract these schemas/owners
	TableFilter   []string `mapstructure:"table_filter"`   // Only extract these tables
	ExcludeSystem bool     `mapstructure:"exclude_system"` // Skip system objects

	// Row count estimation
	IncludeRowCounts bool `mapstructure:"include_row_counts"`
	MaxRowCountTime  int  `mapstructure:"max_row_count_time"` // Max seconds for counting
}

// LogConfig controls logging behavior
type LogConfig struct {
	Level  string `mapstructure:"level"`  // debug, info, warn, error
	Format string `mapstructure:"format"` // json, text
	File   string `mapstructure:"file"`   // log file path (empty = stdout)
}

// Validate performs basic validation on the configuration
func (c *Config) Validate() error {
	if c.Database.Type == "" {
		return ErrMissingDBType
	}
	if c.Output.Format == "" {
		c.Output.Format = "markdown" // default
	}
	if c.Output.OutputDir == "" {
		c.Output.OutputDir = "./output" // default
	}
	return nil
}

// Default returns a configuration with sensible defaults
func Default() *Config {
	return &Config{
		Database: DatabaseConfig{
			Timeout: 30,
			SSLMode: "disable",
		},
		Output: OutputConfig{
			Format:      "markdown",
			OutputDir:   "./output",
			FileName:    "schema_documentation",
			IncludeTOC:  true,
			IncludeERD:  false,
			SplitByType: false,
			Language:    "en",
		},
		Extract: ExtractConfig{
			IncludeTables:    true,
			IncludeViews:     true,
			IncludeRoutines:  true,
			IncludeSequences: true,
			IncludeTriggers:  true,
			IncludeSynonyms:  true,
			IncludeIndexes:   true,
			ExcludeSystem:    true,
			IncludeRowCounts: false,
			MaxRowCountTime:  10,
		},
		Logging: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}
}
