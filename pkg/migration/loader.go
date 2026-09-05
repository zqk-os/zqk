package migration

import (
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// LoadSpec loads a migration spec from a YAML file
func LoadSpec(specPath string) (*Spec, error) {
	data, err := fileutil.ReadFile(specPath)
	if err != nil {
		return nil, errfmt.Newf("failed to read migration spec").Wrap(err)
	}

	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, errfmt.Newf("failed to parse migration spec").Wrap(err)
	}

	// Set defaults
	if spec.SnapshotCompatible {
		if spec.PreMigrationSnapshot == nil {
			spec.PreMigrationSnapshot = &SnapshotConfig{
				Required:   true,
				AutoCreate: true,
			}
		}
		if spec.PostMigrationSnapshot == nil {
			spec.PostMigrationSnapshot = &SnapshotConfig{
				AutoCreate: true,
			}
		}
		if spec.Tracking == nil {
			spec.Tracking = &TrackingConfig{
				ObjectMapping: true,
				FieldMapping:  true,
				StateChanges:  true,
			}
		}
		if spec.Coherence == nil {
			spec.Coherence = &CoherenceConfig{
				AtomicSteps:           true,
				ValidateAfterEachStep: true,
			}
		}
	}

	// Validate spec
	if err := validateSpec(&spec); err != nil {
		return nil, errfmt.Newf("invalid migration spec").Wrap(err)
	}

	return &spec, nil
}

// validateSpec validates a migration spec
func validateSpec(spec *Spec) error {
	if spec.ID == emptyValue {
		return errfmt.Errorf("id is required")
	}
	if spec.Name == emptyValue {
		return errfmt.Errorf("name is required")
	}
	if len(spec.Steps) == 0 {
		return errfmt.Errorf("at least one step is required")
	}

	// Validate step IDs are unique
	stepIDs := make(map[string]bool)
	for _, step := range spec.Steps {
		if step.ID == emptyValue {
			return errfmt.Errorf("step id is required")
		}
		if stepIDs[step.ID] {
			return errfmt.Errorf("duplicate step id: %s", step.ID)
		}
		stepIDs[step.ID] = true
	}

	// Validate step dependencies
	for _, step := range spec.Steps {
		for _, dep := range step.DependsOn {
			if !stepIDs[dep] {
				return errfmt.Errorf("step %s depends on unknown step: %s", step.ID, dep)
			}
		}
		if step.ForEach != emptyValue && !stepIDs[step.ForEach] {
			return errfmt.Errorf("step %s references unknown step in for_each: %s", step.ID, step.ForEach)
		}
	}

	return nil
}
