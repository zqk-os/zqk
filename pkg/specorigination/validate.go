package specorigination

import (
	"regexp"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

var ontologyStemRegexp = regexp.MustCompile(OntologyStemPattern)

// OntologyStemPattern matches valid object_specs ontology stems (lowercase identifier).
const OntologyStemPattern = `^[a-z][a-z0-9_]*$`

// ValidateOptions returns an error when Options cannot be used for spec origination.
func ValidateOptions(o Options) error {
	if o.ProjectRoot == "" {
		return errfmt.Errorf("spec origination: ProjectRoot is required")
	}
	if o.Ontology == "" {
		return errfmt.Errorf("spec origination: Ontology is required")
	}
	if !ontologyStemRegexp.MatchString(o.Ontology) {
		return errfmt.Errorf("spec origination: Ontology %q must match %s", o.Ontology, OntologyStemPattern)
	}
	return nil
}

// authoredFieldValidationSpec scopes origination validation to fields authored
// by the current kind. Inherited fields are validated when their defining
// parent spec is originated, so parent debt cannot block a new child kind.
func authoredFieldValidationSpec(spec *objects.Spec) *objects.Spec {
	if spec == nil {
		return nil
	}
	scoped := *spec
	scoped.ResolvedFields = make(map[string]any, len(spec.Fields))
	for name, field := range spec.Fields {
		scoped.ResolvedFields[name] = field
	}
	return &scoped
}
