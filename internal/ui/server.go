package ui

import (
	"dbms-to-document/internal/exporter"
	"dbms-to-document/internal/model"
	"embed"
	"fmt"
	"html/template"
	"net/http"
)

//go:embed templates/*
var templates embed.FS

// Server provides HTTP endpoints for preview and export
type Server struct {
	schema   *model.Schema
	config   exporter.Config
	template *template.Template
}

// NewServer creates a new UI server
func NewServer(schema *model.Schema, cfg exporter.Config) (*Server, error) {
	// Parse embedded templates
	tmpl, err := template.ParseFS(templates, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Server{
		schema:   schema,
		config:   cfg,
		template: tmpl,
	}, nil
}

// RegisterRoutes registers HTTP handlers
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handlePreview)
	mux.HandleFunc("/export/excel", s.handleExportExcel)
	mux.HandleFunc("/export/word", s.handleExportWord)
}

// handlePreview renders the interactive HTML preview
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := s.template.ExecuteTemplate(w, "preview.html", s.schema); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleExportExcel generates and downloads Excel file
func (s *Server) handleExportExcel(w http.ResponseWriter, r *http.Request) {
	exp, err := exporter.NewExporter("xlsx", s.config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Exporter error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", exp.MimeType())
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"%s_schema%s\"", s.schema.DatabaseName, exp.FileExtension()))

	if err := exp.Export(s.schema, w); err != nil {
		http.Error(w, fmt.Sprintf("Export error: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleExportWord generates and downloads Word document
func (s *Server) handleExportWord(w http.ResponseWriter, r *http.Request) {
	exp, err := exporter.NewExporter("docx", s.config)
	if err != nil {
		http.Error(w, fmt.Sprintf("Exporter error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", exp.MimeType())
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"%s_schema%s\"", s.schema.DatabaseName, exp.FileExtension()))

	if err := exp.Export(s.schema, w); err != nil {
		http.Error(w, fmt.Sprintf("Export error: %v", err), http.StatusInternalServerError)
		return
	}
}

// Start starts the HTTP server
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)

	fmt.Printf("🚀 Preview server starting at http://%s\n", addr)
	fmt.Printf("📊 Database: %s (%s)\n", s.schema.DatabaseName, s.schema.DatabaseType)
	fmt.Printf("📁 Tables: %d | Views: %d | Routines: %d\n",
		len(s.schema.Tables), len(s.schema.Views), len(s.schema.Routines))

	return http.ListenAndServe(addr, mux)
}
