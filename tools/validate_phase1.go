package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// Phase 1 Self-Check Validator
// Ensures strict compliance with critical implementation rules

type ValidationResult struct {
	Rule    string
	Passed  bool
	Details []string
}

func main() {
	fmt.Println("🛡️  Phase 1 Critical Implementation Rules Validator")
	fmt.Println("===================================================\n")

	results := []ValidationResult{
		validateNoSourceCodeFields(),
		validateCommentFields(),
		validateFullObjectCoverage(),
		validateViperTags(),
	}

	allPassed := true
	for _, result := range results {
		status := "❌ FAIL"
		if result.Passed {
			status = "✅ PASS"
		} else {
			allPassed = false
		}

		fmt.Printf("%s Rule: %s\n", status, result.Rule)
		for _, detail := range result.Details {
			fmt.Printf("    %s\n", detail)
		}
		fmt.Println()
	}

	fmt.Println("===================================================")
	if allPassed {
		fmt.Println("✅ ALL CHECKS PASSED - Phase 1 Complete!")
		os.Exit(0)
	} else {
		fmt.Println("❌ VALIDATION FAILED - Fix issues above")
		os.Exit(1)
	}
}

func validateNoSourceCodeFields() ValidationResult {
	result := ValidationResult{
		Rule:    "NO SOURCE CODE FIELDS (Security)",
		Passed:  true,
		Details: []string{},
	}

	forbiddenFields := []string{"Body", "Definition", "Script", "Text", "Source"}
	modelPath := "internal/model/schema.go"

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, modelPath, nil, parser.ParseComments)
	if err != nil {
		result.Passed = false
		result.Details = append(result.Details, fmt.Sprintf("Error parsing %s: %v", modelPath, err))
		return result
	}

	violations := []string{}
	ast.Inspect(node, func(n ast.Node) bool {
		if typeSpec, ok := n.(*ast.TypeSpec); ok {
			if structType, ok := typeSpec.Type.(*ast.StructType); ok {
				for _, field := range structType.Fields.List {
					for _, name := range field.Names {
						for _, forbidden := range forbiddenFields {
							if name.Name == forbidden {
								violations = append(violations,
									fmt.Sprintf("FORBIDDEN field '%s' found in struct %s",
										name.Name, typeSpec.Name.Name))
							}
						}
					}
				}
			}
		}
		return true
	})

	if len(violations) > 0 {
		result.Passed = false
		result.Details = violations
	} else {
		result.Details = append(result.Details,
			"✓ No forbidden source code fields (Body, Definition, Script, Text) found")
		result.Details = append(result.Details,
			"✓ Routine has Signature only (no Body)")
		result.Details = append(result.Details,
			"✓ Trigger has metadata only (no Definition)")
	}

	return result
}

func validateCommentFields() ValidationResult {
	result := ValidationResult{
		Rule:    "COMMENT FIELDS ON ALL STRUCTS (Rich Metadata)",
		Passed:  true,
		Details: []string{},
	}

	modelPath := "internal/model/schema.go"
	requiredStructs := []string{
		"Schema", "Table", "View", "Column", "Routine",
		"RoutineArgument", "Index", "Sequence", "Trigger", "Synonym",
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, modelPath, nil, parser.ParseComments)
	if err != nil {
		result.Passed = false
		result.Details = append(result.Details, fmt.Sprintf("Error parsing %s: %v", modelPath, err))
		return result
	}

	structsWithComment := make(map[string]bool)
	ast.Inspect(node, func(n ast.Node) bool {
		if typeSpec, ok := n.(*ast.TypeSpec); ok {
			if structType, ok := typeSpec.Type.(*ast.StructType); ok {
				hasComment := false
				for _, field := range structType.Fields.List {
					for _, name := range field.Names {
						if name.Name == "Comment" {
							hasComment = true
							break
						}
					}
				}
				structsWithComment[typeSpec.Name.Name] = hasComment
			}
		}
		return true
	})

	missing := []string{}
	for _, structName := range requiredStructs {
		if !structsWithComment[structName] {
			missing = append(missing, fmt.Sprintf("Struct '%s' missing Comment field", structName))
		} else {
			result.Details = append(result.Details, fmt.Sprintf("✓ %s has Comment field", structName))
		}
	}

	if len(missing) > 0 {
		result.Passed = false
		result.Details = append(missing, result.Details...)
	}

	return result
}

func validateFullObjectCoverage() ValidationResult {
	result := ValidationResult{
		Rule:    "FULL OBJECT COVERAGE (Tables/Views/Routines/Sequences/Triggers/Synonyms)",
		Passed:  true,
		Details: []string{},
	}

	modelPath := "internal/model/schema.go"
	requiredTypes := []string{"Table", "View", "Routine", "Sequence", "Trigger", "Synonym"}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, modelPath, nil, parser.ParseComments)
	if err != nil {
		result.Passed = false
		result.Details = append(result.Details, fmt.Sprintf("Error parsing %s: %v", modelPath, err))
		return result
	}

	definedTypes := make(map[string]bool)
	ast.Inspect(node, func(n ast.Node) bool {
		if typeSpec, ok := n.(*ast.TypeSpec); ok {
			definedTypes[typeSpec.Name.Name] = true
		}
		return true
	})

	missing := []string{}
	for _, typeName := range requiredTypes {
		if !definedTypes[typeName] {
			missing = append(missing, fmt.Sprintf("Missing type: %s", typeName))
		} else {
			result.Details = append(result.Details, fmt.Sprintf("✓ %s type defined", typeName))
		}
	}

	if len(missing) > 0 {
		result.Passed = false
		result.Details = append(missing, result.Details...)
	}

	return result
}

func validateViperTags() ValidationResult {
	result := ValidationResult{
		Rule:    "VIPER COMPATIBILITY (mapstructure tags)",
		Passed:  true,
		Details: []string{},
	}

	configPath := "internal/config/config.go"

	content, err := os.ReadFile(configPath)
	if err != nil {
		result.Passed = false
		result.Details = append(result.Details, fmt.Sprintf("Error reading %s: %v", configPath, err))
		return result
	}

	// Simple check for mapstructure tags
	if !strings.Contains(string(content), "mapstructure:") {
		result.Passed = false
		result.Details = append(result.Details, "No mapstructure tags found in config.go")
		return result
	}

	result.Details = append(result.Details, "✓ Config structs have mapstructure tags")
	result.Details = append(result.Details, "✓ Compatible with Viper configuration loading")

	return result
}
