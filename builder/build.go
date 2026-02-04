package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// BuildTarget represents a build target platform
type BuildTarget struct {
	OS   string
	Arch string
}

// Version to embed in binaries
var Version = "1.0.0"

func main() {
	// Get version from command line or use default
	if len(os.Args) > 1 {
		Version = os.Args[1]
	}

	log.Printf("🏗️  pocket-doc Build System")
	log.Printf("Version: %s", Version)
	log.Printf("Build Time: %s", time.Now().Format(time.RFC3339))
	log.Println()

	// Build targets
	targets := []BuildTarget{
		{OS: "windows", Arch: "amd64"},
		{OS: "linux", Arch: "amd64"},
		{OS: "darwin", Arch: "arm64"}, // Mac Apple Silicon
		{OS: "darwin", Arch: "amd64"}, // Mac Intel
	}

	// Create dist directory
	distDir := "dist"
	if err := os.MkdirAll(distDir, 0755); err != nil {
		log.Fatalf("Failed to create dist directory: %v", err)
	}

	// Build for each target
	for _, target := range targets {
		if err := buildTarget(target, distDir); err != nil {
			log.Printf("❌ Failed to build %s/%s: %v", target.OS, target.Arch, err)
			continue
		}
		log.Printf("✅ Built %s/%s", target.OS, target.Arch)
	}

	log.Println()
	log.Printf("🎉 Build complete! Artifacts in ./%s/", distDir)
}

func buildTarget(target BuildTarget, distDir string) error {
	// Binary name
	binaryName := "pocket-doc"
	if target.OS == "windows" {
		binaryName += ".exe"
	}

	// Platform identifier
	platform := fmt.Sprintf("%s-%s", target.OS, target.Arch)

	// Temp build directory
	buildDir := filepath.Join(distDir, "build", platform)
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return fmt.Errorf("failed to create build directory: %w", err)
	}
	defer os.RemoveAll(filepath.Join(distDir, "build"))

	// Build binary
	binaryPath := filepath.Join(buildDir, binaryName)

	log.Printf("🔨 Building %s...", platform)

	// Build command
	cmd := exec.Command("go", "build",
		"-ldflags", fmt.Sprintf("-s -w -X main.Version=%s", Version),
		"-o", binaryPath,
		"./cmd/pocket-doc",
	)

	// Set environment for cross-compilation
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GOOS=%s", target.OS),
		fmt.Sprintf("GOARCH=%s", target.Arch),
		"CGO_ENABLED=0", // Disable CGO for static binaries
	)

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed: %w\nOutput: %s", err, string(output))
	}

	// Copy additional files
	filesToCopy := []string{
		"config.example.yaml",
		"README.md",
	}

	for _, file := range filesToCopy {
		if err := copyFile(file, filepath.Join(buildDir, file)); err != nil {
			log.Printf("⚠️  Warning: Could not copy %s: %v", file, err)
		}
	}

	// Create LICENSE file if it doesn't exist
	licensePath := filepath.Join(buildDir, "LICENSE")
	if err := os.WriteFile(licensePath, []byte(getLicense()), 0644); err != nil {
		log.Printf("⚠️  Warning: Could not create LICENSE: %v", err)
	}

	// Create usage guide
	usagePath := filepath.Join(buildDir, "USAGE.txt")
	if err := os.WriteFile(usagePath, []byte(getUsageGuide(binaryName)), 0644); err != nil {
		log.Printf("⚠️  Warning: Could not create USAGE.txt: %v", err)
	}

	// Create ZIP archive
	zipName := fmt.Sprintf("pocket-doc-%s-%s.zip", Version, platform)
	zipPath := filepath.Join(distDir, zipName)

	if err := createZip(buildDir, zipPath); err != nil {
		return fmt.Errorf("failed to create zip: %w", err)
	}

	// Get file size
	stat, _ := os.Stat(zipPath)
	log.Printf("   📦 Package: %s (%.2f MB)", zipName, float64(stat.Size())/1024/1024)

	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func createZip(sourceDir, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	// Get base directory name for ZIP entries
	baseName := filepath.Base(sourceDir)

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Create ZIP entry with base directory
		zipPath := filepath.Join(baseName, relPath)

		// Use forward slashes in ZIP
		zipPath = strings.ReplaceAll(zipPath, "\\", "/")

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = zipPath
		header.Method = zip.Deflate

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})

	return err
}

func getLicense() string {
	return `MIT License

Copyright (c) 2026 pocket-doc

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
`
}

func getUsageGuide(binaryName string) string {
	cmdPrefix := "./"
	if runtime.GOOS == "windows" {
		cmdPrefix = ""
	}

	return fmt.Sprintf(`pocket-doc Usage Guide
=============================

Quick Start
-----------

1. Copy config.example.yaml to config.yaml
2. Edit config.yaml with your database credentials
3. Run the tool:

   %s%s -mode extract         # Extract schema only
   %s%s -mode export          # Extract and export to file
   %s%s -mode preview         # Extract and start web preview

Command Line Options
--------------------

  -config <file>       Path to configuration file (default: config.yaml)
  -mode <mode>         Operation mode: extract, export, preview (default: extract)
  -format <format>     Export format: xlsx, docx, html (default: xlsx)
  -output <name>       Output filename without extension (default: schema)
  -port <port>         Port for preview server (default: 8080)
  -version             Show version information

Examples
--------

# Extract schema and export to Excel
%s%s -mode export -format xlsx -output mydb_schema

# Extract schema and export to Word
%s%s -mode export -format docx -output mydb_docs

# Extract schema and export to HTML
%s%s -mode export -format html -output mydb_report

# Start interactive preview server
%s%s -mode preview -port 8080

Then open: http://localhost:8080

Supported Databases
-------------------

- Oracle (11g, 12c, 19c, 21c)
- MySQL (5.7, 8.0+)
- PostgreSQL (12+)
- Microsoft SQL Server (2016+)

Security Notes
--------------

- Source code is NEVER extracted (views, procedures, triggers)
- Only metadata and signatures are exported
- Comments and descriptions are included

For more information, visit:
https://github.com/yourusername/pocket-doc
`, cmdPrefix, binaryName, cmdPrefix, binaryName, cmdPrefix, binaryName,
		cmdPrefix, binaryName, cmdPrefix, binaryName, cmdPrefix, binaryName, cmdPrefix, binaryName)
}
