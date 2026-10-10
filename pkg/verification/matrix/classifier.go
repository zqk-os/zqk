package matrix

import (
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultProfile returns the standard, language-agnostic verification profile
// configured with sensible defaults for common languages (.py, .ts, .js, .go, .rs, .md, .sh, etc.).
func DefaultProfile() *Profile {
	return &Profile{
		Name:        "default",
		Description: "Universal multi-language file classification profile",
		Classes: map[FileClass]ClassConfig{
			ClassTest: {
				Name:        ClassTest,
				Description: "Test files, suites, and fixtures across all languages",
				Extensions: []string{
					".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".rs", ".java", ".c", ".cpp", ".cs", ".rb", ".php", ".swift", ".kt",
				},
				MatchPatterns: []string{
					"*_test.*", "*_spec.*", "*.test.*", "*.spec.*", "test_*.*",
					"tests/*", "test/*", "specs/*", "spec/*",
					"**/tests/*", "**/test/*", "**/specs/*", "**/spec/*",
				},
				RequiredChecks: []string{
					"hardcoded-logic",
					"anti-hardcoding-czar",
				},
			},
			ClassSource: {
				Name:        ClassSource,
				Description: "Production source code files (Go, Python, TypeScript, Rust, C, Java, etc.)",
				Extensions: []string{
					".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".rs", ".java", ".c", ".cpp", ".cs", ".rb", ".php", ".swift", ".kt",
				},
				ExcludePatterns: []string{
					"*_test.*", "*_spec.*", "*.test.*", "*.spec.*", "test_*.*",
					"tests/*", "test/*", "specs/*", "spec/*",
					"**/tests/*", "**/test/*", "**/specs/*", "**/spec/*",
				},
				RequiredChecks: []string{
					"hardcoded-logic",
					"anti-hardcoding-czar",
				},
			},
			ClassScript: {
				Name:        ClassScript,
				Description: "Shell scripts, batch files, and automation tooling",
				Extensions:  []string{".sh", ".bash", ".zsh", ".ps1", ".bat", ".cmd"},
				RequiredChecks: []string{
					"hardcoded-logic",
				},
			},
			ClassConfigFile: {
				Name:        ClassConfigFile,
				Description: "Configuration manifests and specifications",
				Extensions:  []string{".yaml", ".yml", ".json", ".toml", ".ini", ".conf", ".cfg"},
				RequiredChecks: []string{
					"hardcoded-logic",
				},
			},
			ClassDocs: {
				Name:           ClassDocs,
				Description:    "Documentation and Markdown guidance",
				Extensions:     []string{".md", ".markdown", ".txt", ".rst", ".adoc"},
				RequiredChecks: []string{},
			},
			ClassOther: {
				Name:           ClassOther,
				Description:    "Unclassified miscellaneous files",
				Extensions:     []string{},
				RequiredChecks: []string{},
			},
		},
	}
}

// DefaultClassConfigs returns standard, language-agnostic classification rules and required checks.
func DefaultClassConfigs() map[FileClass]ClassConfig {
	return DefaultProfile().Classes
}

// Classifier maps repository file paths to their designated FileClass based on a Profile.
type Classifier struct {
	profile *Profile
}

// NewClassifier initializes a classifier with a profile. If profile is nil, DefaultProfile() is used.
func NewClassifier(profile *Profile) *Classifier {
	if profile == nil {
		profile = DefaultProfile()
	}
	return &Classifier{profile: profile}
}

// NewClassifierWithConfigs initializes a classifier with a map of class configurations.
func NewClassifierWithConfigs(configs map[FileClass]ClassConfig) *Classifier {
	if configs == nil {
		configs = DefaultClassConfigs()
	}
	return &Classifier{
		profile: &Profile{
			Name:    "custom",
			Classes: configs,
		},
	}
}

// Profile returns the active Profile backing this classifier.
func (c *Classifier) Profile() *Profile {
	return c.profile
}

// Configs returns the map of class configurations in the classifier's profile.
func (c *Classifier) Configs() map[FileClass]ClassConfig {
	if c.profile == nil {
		return nil
	}
	return c.profile.Classes
}

// Classify determines the FileClass for a relative file path based on configured matchers and extensions.
func (c *Classifier) Classify(relPath string) FileClass {
	if c.profile == nil || len(c.profile.Classes) == 0 {
		return ClassOther
	}

	ext := strings.ToLower(filepath.Ext(relPath))

	// Step 1: Check classes with explicit MatchPatterns first (e.g. tests or specialized classes)
	for class, cfg := range c.profile.Classes {
		if len(cfg.MatchPatterns) == 0 {
			continue
		}
		// If extensions are specified, ensure extension matches
		if len(cfg.Extensions) > 0 {
			extMatches := false
			for _, e := range cfg.Extensions {
				if strings.EqualFold(ext, e) {
					extMatches = true
					break
				}
			}
			if !extMatches {
				continue
			}
		}

		for _, pat := range cfg.MatchPatterns {
			if matchGlob(pat, relPath) {
				return class
			}
		}
	}

	// Step 2: Check classes by extension, respecting ExcludePatterns
	for class, cfg := range c.profile.Classes {
		// Skip if class requires MatchPatterns and didn't match in Step 1
		if len(cfg.MatchPatterns) > 0 {
			continue
		}

		// Check if excluded
		excluded := false
		for _, excl := range cfg.ExcludePatterns {
			if matchGlob(excl, relPath) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		// Check extension match
		for _, e := range cfg.Extensions {
			if strings.EqualFold(ext, e) {
				return class
			}
		}
	}

	return ClassOther
}

func matchGlob(pattern, relPath string) bool {
	normPath := filepath.ToSlash(relPath)
	base := filepath.Base(normPath)

	// Check base filename match with filepath.Match
	if m, err := filepath.Match(pattern, base); err == nil && m {
		return true
	}

	// Check normalized path match with filepath.Match
	if m, err := filepath.Match(pattern, normPath); err == nil && m {
		return true
	}

	// Subpath match: if pattern contains slash (e.g. "tests/*"), also match "*/tests/*"
	if strings.Contains(pattern, "/") && !strings.HasPrefix(pattern, "*") {
		subPattern := "*/" + pattern
		if m, err := filepath.Match(subPattern, normPath); err == nil && m {
			return true
		}
	}

	// Doublestar globbing support (e.g. "**/tests/*")
	if strings.Contains(pattern, "**") {
		rx, err := globToRegexp(pattern)
		if err == nil && rx.MatchString(normPath) {
			return true
		}
	}

	return false
}

func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("^")
	chars := []rune(pattern)
	for i := 0; i < len(chars); i++ {
		c := chars[i]
		switch c {
		case '*':
			if i+1 < len(chars) && chars[i+1] == '*' {
				sb.WriteString(".*")
				i++
				if i+1 < len(chars) && chars[i+1] == '/' {
					sb.WriteString("(?:/)?")
					i++
				}
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '{', '}', '[', ']', '^', '$', '\\':
			sb.WriteString("\\")
			sb.WriteRune(c)
		default:
			sb.WriteRune(c)
		}
	}
	sb.WriteString("$")
	return regexp.Compile(sb.String())
}
