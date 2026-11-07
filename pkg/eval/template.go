package eval

import (
	"math"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

const (
	defaultSchemaVersion = 1
	expectedKind         = "eval_template"
	weightTolerance      = 0.01
	minThreshold         = 1.0
	maxThreshold         = 5.0
)

// LoadTemplate reads, parses, and validates an EvalTemplate from the filesystem.
func LoadTemplate(path string) (*EvalTemplate, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil, errfmt.Errorf("eval template path cannot be empty")
	}

	data, err := fileutil.ReadFile(cleanPath)
	if err != nil {
		return nil, errfmt.Errorf("failed to read eval template %s: %w", cleanPath, err)
	}

	var tmpl EvalTemplate
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return nil, errfmt.Errorf("failed to parse eval template YAML from %s: %w", cleanPath, err)
	}

	if err := ValidateTemplate(&tmpl); err != nil {
		return nil, errfmt.Errorf("invalid eval template %s: %w", cleanPath, err)
	}

	return &tmpl, nil
}

// ValidateTemplate enforces structural and invariant validation on an EvalTemplate.
func ValidateTemplate(tmpl *EvalTemplate) error {
	if tmpl == nil {
		return errfmt.Errorf("eval template cannot be nil")
	}

	if tmpl.SchemaVersion <= 0 {
		tmpl.SchemaVersion = defaultSchemaVersion
	}

	if tmpl.Kind != "" && tmpl.Kind != expectedKind {
		return errfmt.Errorf("invalid kind %q, expected %q", tmpl.Kind, expectedKind)
	}

	if strings.TrimSpace(tmpl.TemplateID) == "" {
		return errfmt.Errorf("template_id is required and cannot be blank")
	}

	if strings.TrimSpace(tmpl.Title) == "" {
		return errfmt.Errorf("title is required and cannot be blank")
	}

	if len(tmpl.Dimensions) > 0 {
		var totalWeight float64
		for dimName, dimSpec := range tmpl.Dimensions {
			if strings.TrimSpace(dimName) == "" {
				return errfmt.Errorf("dimension name cannot be blank")
			}
			if dimSpec.Weight < 0.0 || dimSpec.Weight > 1.0 {
				return errfmt.Errorf("dimension %s weight %0.2f must be in range [0.0, 1.0]", dimName, dimSpec.Weight)
			}
			if dimSpec.Threshold < minThreshold || dimSpec.Threshold > maxThreshold {
				return errfmt.Errorf("dimension %s threshold %0.2f must be in range [1.0, 5.0]", dimName, dimSpec.Threshold)
			}
			totalWeight += dimSpec.Weight
		}

		if math.Abs(totalWeight-1.0) > weightTolerance {
			return errfmt.Errorf("dimension weights must sum to 1.0 (got %0.2f)", totalWeight)
		}
	}

	return nil
}
