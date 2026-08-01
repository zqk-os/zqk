package specorigination

import (
	"regexp"

	"github.com/lanceman/zqk/pkg/errfmt"
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
