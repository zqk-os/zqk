package objects

import (
	"context"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

func (sl *SpecLoader) specStamp(absPath string) stampmemo.Stamp {
	return stampmemo.Of(absPath)
}

func (sl *SpecLoader) absSpecPath(specFile string) string {
	specPath := sl.resolveSpecFilePath(specFile)
	if abs, err := filepath.Abs(specPath); err == nil {
		return abs
	}
	return specPath
}

func (sl *SpecLoader) readCachedBytes(absPath string) ([]byte, error) {
	return sl.fileBytes.Load(absPath, sl.specStamp(absPath), func() ([]byte, error) {
		data, _, err := sl.readSpecFile(context.Background(), absPath)
		return data, err
	})
}

func (sl *SpecLoader) invalidateOntology(ontology string) {
	if ontology == emptyValue || ontology == "null" {
		return
	}
	sl.specsByOntology.Delete(ontology)
	absPath := sl.absSpecPath(ontology + ".yaml")
	sl.specsByPath.Delete(absPath)
	sl.fileBytes.Delete(absPath)
	sl.ontologies.Delete(absPath)
}

// LoadSpecWithInheritance loads a spec and resolves its inheritance chain.
func (sl *SpecLoader) LoadSpecWithInheritance(specFile string) (*Spec, error) {
	return sl.loadSpecCached(specFile, make(map[string]bool), 0)
}

func (sl *SpecLoader) loadSpecCached(specFile string, visited map[string]bool, depth int) (*Spec, error) {
	specPath := sl.absSpecPath(specFile)
	if specPath != emptyValue && visited[specPath] {
		return nil, errfmt.Errorf("circular inheritance detected: %s", specFile)
	}
	if specPath != emptyValue {
		visited[specPath] = true
	}
	spec, err := sl.specsByPath.Load(specPath, sl.specStamp(specPath), func() (*Spec, error) {
		return sl.loadSpecWithInheritanceRecursive(specFile, visited, "", depth)
	})
	if err != nil {
		return nil, err
	}
	if spec != nil && spec.Ontology != emptyValue {
		_, _ = sl.specsByOntology.Load(spec.Ontology, sl.specStamp(specPath), func() (*Spec, error) {
			return spec, nil
		})
	}
	return spec, nil
}

func (sl *SpecLoader) getOntologyFromFile(filePath string) string {
	name, err := sl.ontologies.Load(filePath, sl.specStamp(filePath), func() (string, error) {
		data, err := sl.readCachedBytes(filePath)
		if err != nil {
			return "", err
		}
		var partial struct {
			Ontology string `yaml:"ontology"`
		}
		if err := yaml.Unmarshal(data, &partial); err != nil {
			return "", err
		}
		return partial.Ontology, nil
	})
	if err != nil {
		return ""
	}
	return name
}

// ClearCache drops every spec memo because the key space was explicitly invalidated.
// This is not a size cap — see pkg/stampmemo.
func (sl *SpecLoader) ClearCache() {
	sl.specsByPath.Reset()
	sl.specsByOntology.Reset()
	sl.ontologies.Reset()
	sl.fileBytes.Reset()
	sl.bumpSpecCacheRevision()
}

// InvalidateSpec invalidates a specific spec entry by ontology.
func (sl *SpecLoader) InvalidateSpec(ontology string) {
	sl.invalidateOntology(ontology)
	sl.bumpSpecCacheRevision()
}

// InvalidateSpecByFile invalidates a spec entry by file path.
func (sl *SpecLoader) InvalidateSpecByFile(filePath string) {
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(sl.specsDir, filePath)
	}
	if abs, err := filepath.Abs(absPath); err == nil {
		absPath = abs
	}
	sl.specsByOntology.Delete(strings.TrimSuffix(filepath.Base(absPath), ".yaml"))
	sl.specsByPath.Delete(absPath)
	sl.ontologies.Delete(absPath)
	sl.fileBytes.Delete(absPath)
	sl.bumpSpecCacheRevision()
}
