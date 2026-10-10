package matrix

import (
	"path/filepath"
	"strings"
)

// DefaultClassConfigs returns standard classification rules and required checks.
func DefaultClassConfigs() map[FileClass]ClassConfig {
	return map[FileClass]ClassConfig{
		ClassGoProd: {
			Name:            ClassGoProd,
			Description:     "Production Go source code files",
			Extensions:      []string{".go"},
			ExcludePatterns: []string{"*_test.go"},
			RequiredChecks: []string{
				"hardcoded-logic",
				"ast-hygiene",
				"anti-hardcoding-czar",
			},
		},
		ClassGoTest: {
			Name:          ClassGoTest,
			Description:   "Go test files and test suites",
			Extensions:    []string{".go"},
			MatchPatterns: []string{"*_test.go"},
			RequiredChecks: []string{
				"hardcoded-logic",
				"ast-hygiene",
			},
		},
		ClassScript: {
			Name:        ClassScript,
			Description: "Shell scripts and automation tooling",
			Extensions:  []string{".sh", ".bash"},
			RequiredChecks: []string{
				"hardcoded-logic",
			},
		},
		ClassConfigFile: {
			Name:        ClassConfigFile,
			Description: "Configuration manifests and specifications",
			Extensions:  []string{".yaml", ".yml", ".json", ".toml"},
			RequiredChecks: []string{
				"hardcoded-logic",
			},
		},
		ClassDocs: {
			Name:           ClassDocs,
			Description:    "Documentation and Markdown guidance",
			Extensions:     []string{".md", ".markdown", ".txt"},
			RequiredChecks: []string{},
		},
		ClassOther: {
			Name:           ClassOther,
			Description:    "Unclassified miscellaneous files",
			Extensions:     []string{},
			RequiredChecks: []string{},
		},
	}
}

// Classifier maps repository file paths to their designated FileClass.
type Classifier struct {
	configs map[FileClass]ClassConfig
}

// NewClassifier initializes a classifier with default or custom class configurations.
func NewClassifier(configs map[FileClass]ClassConfig) *Classifier {
	if configs == nil {
		configs = DefaultClassConfigs()
	}
	return &Classifier{configs: configs}
}

// Classify determines the FileClass for a relative file path.
func (c *Classifier) Classify(relPath string) FileClass {
	base := filepath.Base(relPath)
	ext := strings.ToLower(filepath.Ext(relPath))

	// 1. Check Go Test first
	if ext == ".go" && strings.HasSuffix(base, "_test.go") {
		return ClassGoTest
	}

	// 2. Check Go Production
	if ext == ".go" {
		return ClassGoProd
	}

	// 3. Check Scripts
	if ext == ".sh" || ext == ".bash" {
		return ClassScript
	}

	// 4. Check Configs
	if ext == ".yaml" || ext == ".yml" || ext == ".json" || ext == ".toml" {
		return ClassConfigFile
	}

	// 5. Check Docs
	if ext == ".md" || ext == ".markdown" || ext == ".txt" {
		return ClassDocs
	}

	// 6. Custom match patterns across configured classes
	for class, cfg := range c.configs {
		for _, pat := range cfg.MatchPatterns {
			matched, err := filepath.Match(pat, base)
			if err == nil && matched {
				return class
			}
		}
	}

	return ClassOther
}
