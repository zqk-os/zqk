package kindnames

import (
	"errors"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	emptyValue            = ""
	errSpecsDirEmpty      = "specs directory is empty"
	placeholderSpecName   = "_placeholder.yaml"
	yamlExt               = ".yaml"
	ymlExt                = ".yml"
	errNoOntologyTemplate = "no ontology values found under %s"
)

type specOntologyHeader struct {
	Ontology string `yaml:"ontology"`
}

var kindNameSets stampmemo.Table[map[string]struct{}] // keyed by specsDir; stamp is top-level YAML files

// LoadKindNamesFromSpecsDir walks specsDir for *.yaml / *.yml and returns unique ontology
// strings (object kind names) from each file’s top-level `ontology` field.
//
// Used by drift analysis, future codegen, and any tool that needs the spec-known kind set
// without duplicating directory-walk logic. When a SpecIndex is already loaded (e.g. from
// spec_index.json), prefer objects.KindNamesFromSpecIndex so the kind set matches the
// materialized index and participates in the same snapshot story as the spec origin plane.
func LoadKindNamesFromSpecsDir(specsDir string) (map[string]struct{}, error) {
	if specsDir == emptyValue {
		return nil, errors.New(errSpecsDirEmpty)
	}
	files, err := listSpecYAMLFiles(specsDir)
	if err != nil {
		return nil, err
	}
	stamp := stampmemo.OfAll(append([]string{specsDir}, files...)...)
	out, loadErr := kindNameSets.Load(specsDir, stamp, func() (map[string]struct{}, error) {
		return readKindNames(files, specsDir)
	})
	if loadErr != nil {
		return nil, loadErr
	}
	return out, nil
}

func listSpecYAMLFiles(specsDir string) ([]string, error) {
	entries, err := fileutil.ReadDir(specsDir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if appledouble.SkipNameInReadDir(name) || name == placeholderSpecName {
			continue
		}
		if !strings.HasSuffix(name, yamlExt) && !strings.HasSuffix(name, ymlExt) {
			continue
		}
		files = append(files, filepath.Join(specsDir, name))
	}
	return files, nil
}

func readKindNames(files []string, specsDir string) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	for _, path := range files {
		data, err := fileutil.ReadFile(path)
		if err != nil {
			continue
		}
		var hdr specOntologyHeader
		if err := yaml.Unmarshal(data, &hdr); err != nil {
			continue
		}
		if hdr.Ontology != emptyValue {
			out[hdr.Ontology] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil, errfmt.Errorf(errNoOntologyTemplate, specsDir)
	}
	return out, nil
}
