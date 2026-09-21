package authcred

import (
	"errors"
	"slices"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"gopkg.in/yaml.v3"
)

type personaVocabFile struct {
	VocabularySchemeRefs []string `yaml:"vocabulary_scheme_refs"`
}

var (
	personaVocab    stampmemo.View[[]string]
	errPersonaVocab = errors.New("persona vocabulary yaml")
)

// VocabularySchemesForPersona returns vocabulary_scheme_refs for a persona id.
// Kind dir and listing-index paths come from the path alias cache; parsed YAML
// is retained until those files' mtimes change.
func VocabularySchemesForPersona(projectRoot, personaID string) []string {
	personaID = strings.TrimSpace(personaID)
	if projectRoot == "" || personaID == "" {
		return nil
	}
	raw, ok := casYAML(projectRoot, personaID, paths.PersonaIndexPath(projectRoot), paths.PersonasDirPath(projectRoot))
	if !ok {
		return nil
	}
	refs, err := personaVocab.Get(projectRoot+"\x00"+personaID, raw, func(raw []byte) ([]string, error) {
		var body personaVocabFile
		if yaml.Unmarshal(raw, &body) != nil {
			return nil, errPersonaVocab
		}
		return slices.Clone(body.VocabularySchemeRefs), nil
	})
	if err != nil {
		return nil
	}
	return slices.Clone(refs)
}
