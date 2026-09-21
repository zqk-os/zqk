package authcred

import (
	"slices"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

type personaVocabFile struct {
	VocabularySchemeRefs []string `yaml:"vocabulary_scheme_refs"`
}

type personaVocabHit struct {
	raw  []byte
	refs []string
}

var personaVocabParsed sync.Map // projectRoot + "\x00" + personaID -> personaVocabHit

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
	cacheKey := projectRoot + "\x00" + personaID
	if hit, ok := personaVocabParsed.Load(cacheKey); ok {
		parsed := hit.(personaVocabHit)
		if sameByteBacking(parsed.raw, raw) {
			return slices.Clone(parsed.refs)
		}
	}
	var body personaVocabFile
	if yaml.Unmarshal(raw, &body) != nil {
		return nil
	}
	personaVocabParsed.Store(cacheKey, personaVocabHit{raw: raw, refs: slices.Clone(body.VocabularySchemeRefs)})
	return slices.Clone(body.VocabularySchemeRefs)
}
