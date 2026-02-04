package exporter

import (
	"dbms-to-document/internal/exporter/docx"
	"dbms-to-document/internal/exporter/html"
	"dbms-to-document/internal/exporter/xlsx"
	"fmt"
	"strings"
)

// NewExporter creates an exporter for the specified format
// Use format-specific config structs (xlsx.Config or docx.Config)
func NewExporter(format string, cfg Config) (Exporter, error) {
	format = strings.ToLower(strings.TrimSpace(format))

	switch format {
	case "xlsx", "excel":
		xlsxCfg := xlsx.Config{
			Language:     cfg.Language,
			ExcludeTypes: cfg.ExcludeTypes,
			ColorScheme:  cfg.ColorScheme,
		}
		return xlsx.NewExporter(xlsxCfg), nil
	case "docx", "word":
		docxCfg := docx.Config{
			Language:         cfg.Language,
			IncludeTOC:       cfg.IncludeTOC,
			IncludeCoverPage: cfg.IncludeCoverPage,
			CompanyName:      cfg.CompanyName,
			ProjectName:      cfg.ProjectName,
			Author:           cfg.Author,
			ExcludeTypes:     cfg.ExcludeTypes,
			ColorScheme:      cfg.ColorScheme,
		}
		return docx.NewExporter(docxCfg), nil
	case "html":
		htmlCfg := html.Config{
			Language: cfg.Language,
			Title:    "Schema Documentation",
		}
		return html.NewExporter(htmlCfg), nil
	default:
		return nil, fmt.Errorf("unsupported export format: %s (supported: xlsx, docx, html)", format)
	}
}

// GetSupportedFormats returns a list of supported export formats
func GetSupportedFormats() []string {
	return []string{"xlsx", "docx", "html"}
}
