package main

import (
	"context"
	"pocket-doc/internal/config"
	"pocket-doc/internal/exporter"
	"pocket-doc/internal/extractor"
	"pocket-doc/internal/ui"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// Version will be set during build with -ldflags
var Version = "dev"

func main() {
	// Command line flags
	configFile := flag.String("config", "config.yaml", "Path to configuration file")
	mode := flag.String("mode", "extract", "Mode: extract, preview, or export")
	format := flag.String("format", "xlsx", "Export format: xlsx, docx, html")
	output := flag.String("output", "schema", "Output file name (without extension)")
	port := flag.String("port", "8080", "Port for preview server")
	version := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *version {
		fmt.Printf("pocket-doc %s\n", Version)
		return
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create database extractor
	extractorConfig := extractor.Config{
		Host:         cfg.Database.Host,
		Port:         cfg.Database.Port,
		Database:     cfg.Database.Database,
		Username:     cfg.Database.Username,
		Password:     cfg.Database.Password,
		SSLMode:      cfg.Database.SSLMode,
		SchemaFilter: cfg.Database.SchemaFilter,
	}

	ext, err := extractor.NewDBExtractor(cfg.Database.Type, extractorConfig)
	if err != nil {
		log.Fatalf("Failed to create extractor: %v", err)
	}
	defer ext.Close()

	// Connect to database
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("Connecting to %s database at %s:%d...", cfg.Database.Type, cfg.Database.Host, cfg.Database.Port)
	if err := ext.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Extract schema
	log.Println("Extracting schema metadata...")
	schema, err := ext.ExtractSchema(ctx)
	if err != nil {
		log.Fatalf("Failed to extract schema: %v", err)
	}

	log.Printf("✅ Extraction complete: %d tables, %d views, %d routines",
		len(schema.Tables), len(schema.Views), len(schema.Routines))

	// Execute based on mode
	switch *mode {
	case "extract":
		// Just extraction - already done
		log.Println("✅ Extraction complete")

	case "export":
		// Export to file
		exportConfig := exporter.Config{
			Language:         cfg.Output.Language,
			IncludeTOC:       cfg.Output.IncludeTOC,
			IncludeCoverPage: cfg.Output.IncludeCoverPage,
			CompanyName:      cfg.Output.CompanyName,
			ProjectName:      cfg.Output.ProjectName,
			Author:           cfg.Output.Author,
			ColorScheme:      cfg.Output.ColorScheme,
		}

		exp, err := exporter.NewExporter(*format, exportConfig)
		if err != nil {
			log.Fatalf("Failed to create exporter: %v", err)
		}

		filename := fmt.Sprintf("%s%s", *output, exp.FileExtension())
		f, err := os.Create(filename)
		if err != nil {
			log.Fatalf("Failed to create output file: %v", err)
		}
		defer f.Close()

		log.Printf("Exporting to %s...", filename)
		if err := exp.Export(schema, f); err != nil {
			log.Fatalf("Failed to export: %v", err)
		}

		log.Printf("✅ Export complete: %s", filename)

	case "preview":
		// Start web server for preview
		exportConfig := exporter.Config{
			Language:         cfg.Output.Language,
			IncludeTOC:       cfg.Output.IncludeTOC,
			IncludeCoverPage: cfg.Output.IncludeCoverPage,
			CompanyName:      cfg.Output.CompanyName,
			ProjectName:      cfg.Output.ProjectName,
			Author:           cfg.Output.Author,
			ColorScheme:      cfg.Output.ColorScheme,
		}

		server, err := ui.NewServer(schema, exportConfig)
		if err != nil {
			log.Fatalf("Failed to create UI server: %v", err)
		}

		mux := http.NewServeMux()
		server.RegisterRoutes(mux)

		addr := ":" + *port
		log.Printf("🌐 Preview server starting at http://localhost%s", addr)
		log.Println("   - Preview: http://localhost" + addr)
		log.Println("   - Export Excel: http://localhost" + addr + "/export/excel")
		log.Println("   - Export Word: http://localhost" + addr + "/export/word")

		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Fatalf("Server error: %v", err)
		}

	default:
		log.Fatalf("Unknown mode: %s (use: extract, export, or preview)", *mode)
	}
}
