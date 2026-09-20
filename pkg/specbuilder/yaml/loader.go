package yaml

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/appledouble"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/specbuilder/core"
)

const emptyValue = ""

// YAMLSpecLoader is a generic YAML spec loader
// It can load specs that implement core.Spec from YAML files
type YAMLSpecLoader[S core.Spec] struct {
	baseDir         string
	schemaValidator *SchemaValidator
	validateSchema  bool
}

// NewYAMLSpecLoader creates a new YAML spec loader
func NewYAMLSpecLoader[S core.Spec](baseDir string) *YAMLSpecLoader[S] {
	return &YAMLSpecLoader[S]{
		baseDir:        baseDir,
		validateSchema: true, // Enable schema validation by default
	}
}

// WithSchemaValidation enables or disables JSON schema validation
func (sl *YAMLSpecLoader[S]) WithSchemaValidation(enabled bool) *YAMLSpecLoader[S] {
	sl.validateSchema = enabled
	return sl
}

// WithSchemaValidator sets a custom schema validator
func (sl *YAMLSpecLoader[S]) WithSchemaValidator(validator *SchemaValidator) *YAMLSpecLoader[S] {
	sl.schemaValidator = validator
	return sl
}

// getSchemaValidator returns the schema validator, creating one if needed
func (sl *YAMLSpecLoader[S]) getSchemaValidator() *SchemaValidator {
	if sl.schemaValidator == nil {
		// Default schemas directory: .zqk/cli/specs/schemas (relative to project root)
		// Try to find project root by looking for .zqk directory
		schemasDir := ".zqk/cli/specs/schemas"
		// Try to resolve from baseDir
		if sl.baseDir != emptyValue {
			// Look for .zqk directory starting from baseDir
			dir := sl.baseDir
			for i := 0; i < 10; i++ { // Limit depth
				testPath := filepath.Join(dir, paths.ProjectDataDir, "cli", "specs", "schemas")
				if _, err := fileutil.Stat(testPath); err == nil {
					schemasDir = testPath
					break
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					break
				}
				dir = parent
			}
		}
		sl.schemaValidator = NewSchemaValidator(schemasDir)
	}
	return sl.schemaValidator
}

// LoadSpec loads a single spec from a YAML file
// T is the spec type that implements core.Spec
func LoadYAMLSpec[S core.Spec](filePath string) (S, error) {
	loader := NewYAMLSpecLoader[S]("")
	return loader.LoadSpec(filePath)
}

// LoadSpec loads a single spec from a YAML file with schema validation
func (sl *YAMLSpecLoader[S]) LoadSpec(filePath string) (S, error) {
	var spec S

	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return spec, errfmt.Newf("failed to read spec file").Wrap(err)
	}

	// Validate against JSON schema if enabled
	if sl.validateSchema {
		validator := sl.getSchemaValidator()
		if err := validator.ValidateYAMLWithAutoSchema(filePath); err != nil {
			// Log warning but don't fail (schema validation is optional for now)
			// In the future, this could be made strict
			// For now, we'll log but continue
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("Schema validation failed for YAML spec").
				String("file_path", filePath).
				WithError(err).
				Log()
			// Uncomment to make schema validation strict:
			// return spec, errfmt.Newf("schema validation failed").Wrap(err)
		}
	}

	if err := yaml.Unmarshal(data, &spec); err != nil {
		return spec, errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Validate spec
	if err := spec.Validate(); err != nil {
		return spec, errfmt.Newf("spec validation failed").Wrap(err)
	}

	return spec, nil
}

// LoadYAMLSpecs loads multiple specs from a YAML file containing a list
func LoadYAMLSpecs[S core.Spec](filePath string) ([]S, error) {
	loader := NewYAMLSpecLoader[S]("")
	return loader.LoadSpecs(filePath)
}

// LoadSpecs loads multiple specs from a YAML file containing a list
func (sl *YAMLSpecLoader[S]) LoadSpecs(filePath string) ([]S, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read spec file").Wrap(err)
	}

	// Validate against JSON schema if enabled
	if sl.validateSchema {
		validator := sl.getSchemaValidator()
		if err := validator.ValidateYAMLWithAutoSchema(filePath); err != nil {
			// Log warning but don't fail (schema validation is optional for now)
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("Schema validation failed for YAML specs").
				String("file_path", filePath).
				WithError(err).
				Log()
		}
	}

	var specs []S
	if err := yaml.Unmarshal(data, &specs); err != nil {
		return nil, errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Validate all specs
	for i, spec := range specs {
		if err := spec.Validate(); err != nil {
			return nil, errfmt.Errorf("spec %d validation failed: %w", i, err)
		}
	}

	return specs, nil
}

// LoadYAMLSpecList loads specs from a YAML file with a "specs" or "scenarios" key
// This is useful for files like scenarios.yaml that have a top-level key
func LoadYAMLSpecList[S core.Spec](filePath string, listKey string) ([]S, error) {
	loader := NewYAMLSpecLoader[S]("")
	return loader.LoadSpecList(filePath, listKey)
}

// LoadSpecList loads specs from a YAML file with a "specs" or "scenarios" key
func (sl *YAMLSpecLoader[S]) LoadSpecList(filePath string, listKey string) ([]S, error) {
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read spec file").Wrap(err)
	}

	// Validate against JSON schema if enabled
	if sl.validateSchema {
		validator := sl.getSchemaValidator()
		if err := validator.ValidateYAMLWithAutoSchema(filePath); err != nil {
			// Log warning but don't fail (schema validation is optional for now)
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("Schema validation failed for YAML spec list").
				String("file_path", filePath).
				String("list_key", listKey).
				WithError(err).
				Log()
		}
	}

	var wrapper map[string]any
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	listData, ok := wrapper[listKey]
	if !ok {
		return nil, errfmt.Errorf("key %s not found in YAML file", listKey)
	}

	// Re-marshal the list to get proper type
	listBytes, err := yaml.Marshal(listData)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal list").Wrap(err)
	}

	var specs []S
	if err := yaml.Unmarshal(listBytes, &specs); err != nil {
		return nil, errfmt.Newf("failed to parse spec list").Wrap(err)
	}

	// Validate all specs
	for i, spec := range specs {
		if err := spec.Validate(); err != nil {
			return nil, errfmt.Errorf("spec %d validation failed: %w", i, err)
		}
	}

	return specs, nil
}

// FindSpecFiles finds all YAML files in a directory that might contain specs
func FindSpecFiles(dir string, pattern string) ([]string, error) {
	if pattern == emptyValue {
		pattern = "*.yaml"
	}

	var files []string
	err := filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			// Skip macOS AppleDouble/resource-fork files (._*)
			if appledouble.SkipNameInReadDir(info.Name()) {
				return nil
			}
			matched, err := filepath.Match(pattern, info.Name())
			if err != nil {
				return err
			}
			if matched {
				files = append(files, path)
			}
		}

		return nil
	})

	return files, err
}
