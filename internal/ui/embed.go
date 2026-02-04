package ui

import "embed"

// Templates embeds all HTML templates for the UI
//
//go:embed templates/*.html
var Templates embed.FS

// Static embeds static assets (CSS, JS, images)
// Uncomment and create static/ directory if you add static files
//
// //go:embed static/*
// var Static embed.FS
