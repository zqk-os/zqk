package semantic

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// findFiles finds files with given extensions, respecting project ignores.
func findFiles(root string, extensions []string) []string {
	var files []string

	_ = filepath.Walk(root, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			// Skip common directories
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && base != "." {
				return filepath.SkipDir
			}
			if base == "node_modules" || base == "vendor" || base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}

		for _, ext := range extensions {
			if strings.HasSuffix(strings.ToLower(path), ext) {
				relPath, err := filepath.Rel(root, path)
				if err == nil {
					files = append(files, relPath)
				}
			}
		}

		return nil
	})

	return files
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// StructuredDataScanner checks for structured data files (YAML, JSON).
type StructuredDataScanner struct{}

func (s *StructuredDataScanner) Scan(projectRoot string) ([]Indicator, error) {
	indicator := Indicator{
		Type:           "structured_data",
		Found:          false,
		Examples:       []string{},
		Sophistication: "none",
	}

	// Check for YAML files
	yamlFiles := findFiles(projectRoot, []string{".yaml", ".yml"})
	if len(yamlFiles) > 0 {
		indicator.Found = true
		indicator.Examples = append(indicator.Examples, yamlFiles[:minInt(3, len(yamlFiles))]...)
		indicator.Sophistication = "basic"
	}

	// Check for JSON files
	jsonFiles := findFiles(projectRoot, []string{".json"})
	if len(jsonFiles) > 0 {
		indicator.Found = true
		if len(indicator.Examples) < 3 {
			indicator.Examples = append(indicator.Examples, jsonFiles[:minInt(3-len(indicator.Examples), len(jsonFiles))]...)
		}
		if indicator.Sophistication == "none" {
			indicator.Sophistication = "basic"
		}
	}

	if !indicator.Found {
		return nil, nil
	}

	return []Indicator{indicator}, nil
}

// SchemaScanner checks for formal schema files (JSON Schema, XSD).
type SchemaScanner struct{}

func (s *SchemaScanner) Scan(projectRoot string) ([]Indicator, error) {
	indicator := Indicator{
		Type:           "formal_schemas",
		Found:          false,
		Examples:       []string{},
		Sophistication: "none",
	}

	// Check for JSON Schema files
	jsonSchemaFiles := findFiles(projectRoot, []string{"schema.json", ".schema.json"})
	// Check for XSD files
	xsdFiles := findFiles(projectRoot, []string{".xsd"})

	if len(jsonSchemaFiles) > 0 || len(xsdFiles) > 0 {
		indicator.Found = true
		allFiles := make([]string, 0, len(jsonSchemaFiles)+len(xsdFiles))
		allFiles = append(allFiles, jsonSchemaFiles...)
		allFiles = append(allFiles, xsdFiles...)
		indicator.Examples = allFiles[:minInt(3, len(allFiles))]
		indicator.Sophistication = "intermediate"
	}

	if !indicator.Found {
		return nil, nil
	}

	return []Indicator{indicator}, nil
}

// OntologyScanner checks for ontology files (RDF, OWL, TTL, N3).
type OntologyScanner struct{}

func (s *OntologyScanner) Scan(projectRoot string) ([]Indicator, error) {
	indicator := Indicator{
		Type:           "ontologies",
		Found:          false,
		Examples:       []string{},
		Sophistication: "none",
	}

	// Check for RDF files
	rdfFiles := findFiles(projectRoot, []string{".rdf", ".owl", ".ttl", ".n3"})
	if len(rdfFiles) > 0 {
		indicator.Found = true
		indicator.Examples = rdfFiles[:minInt(3, len(rdfFiles))]
		indicator.Sophistication = "advanced"
	}

	if !indicator.Found {
		return nil, nil
	}

	return []Indicator{indicator}, nil
}

// SemanticRepositoryScanner checks for semantic repository configurations.
type SemanticRepositoryScanner struct{}

func (s *SemanticRepositoryScanner) Scan(projectRoot string) ([]Indicator, error) {
	indicator := Indicator{
		Type:           "semantic_repositories",
		Found:          false,
		Examples:       []string{},
		Sophistication: "none",
	}

	// Check for SPARQL endpoint configurations
	configFiles := []string{
		filepath.Join(paths.ProjectDataDir, paths.ProjectConfigFile),
		"config.yaml",
		".env",
		"docker-compose.yml",
	}

	for _, configFile := range configFiles {
		path := filepath.Join(projectRoot, configFile)
		if _, err := fileutil.Stat(path); err == nil {
			content, err := fileutil.ReadFile(path)
			if err == nil {
				contentStr := strings.ToLower(string(content))
				if strings.Contains(contentStr, "sparql") ||
					strings.Contains(contentStr, "graphdb") ||
					strings.Contains(contentStr, "fuseki") ||
					strings.Contains(contentStr, "virtuoso") {
					indicator.Found = true
					indicator.Examples = append(indicator.Examples, configFile)
					indicator.Sophistication = "expert"
					break
				}
			}
		}
	}

	if !indicator.Found {
		return nil, nil
	}

	return []Indicator{indicator}, nil
}
