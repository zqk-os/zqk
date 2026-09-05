package objects

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

// mergeResolvedOverlay copies src resolved fields, traits, and exclude_traits into dst.
// Existing dst keys win (child / earlier overlay). Storage profile fills only if dst is empty.
func mergeResolvedOverlay(dst, src *Spec) {
	if dst == nil || src == nil {
		return
	}
	if dst.ResolvedFields == nil {
		dst.ResolvedFields = make(map[string]any)
	}
	for k, v := range src.ResolvedFields {
		if _, exists := dst.ResolvedFields[k]; !exists {
			dst.ResolvedFields[k] = v
		}
	}
	seen := make(map[string]bool, len(dst.ResolvedTraits))
	for _, t := range dst.ResolvedTraits {
		seen[t] = true
	}
	for _, t := range src.ResolvedTraits {
		if t != emptyValue && !seen[t] {
			dst.ResolvedTraits = append(dst.ResolvedTraits, t)
			seen[t] = true
		}
	}
	excluded := make(map[string]bool, len(dst.ExcludeTraits))
	for _, t := range dst.ExcludeTraits {
		excluded[t] = true
	}
	for _, t := range src.ExcludeTraits {
		if t != emptyValue && !excluded[t] {
			dst.ExcludeTraits = append(dst.ExcludeTraits, t)
			excluded[t] = true
		}
	}
	if dst.StorageProfile == "" && src.StorageProfile != "" {
		dst.StorageProfile = src.StorageProfile
	}
}

// mergeComposes overlays each composed mixin after extends.
// Each mixin is loaded with a fresh extends-visited map so composing occupancy
// (work_interval chain) does not false-cycle against a leaf that already walked
// work_unit → work_interval.
func (sl *SpecLoader) mergeComposes(spec *Spec, depth int) error {
	if spec == nil || sl == nil {
		return nil
	}
	for _, mixin := range spec.Composes {
		mixin = strings.TrimSpace(mixin)
		if mixin == emptyValue || mixin == "null" {
			continue
		}
		mixinSpec, err := sl.loadComposeMixin(mixin, depth+1)
		if err != nil {
			return errfmt.Errorf("failed to load composed spec %s for %s: %w", mixin, spec.Ontology, err)
		}
		mergeResolvedOverlay(spec, mixinSpec)
	}
	return nil
}

func (sl *SpecLoader) loadComposeMixin(ontology string, depth int) (*Spec, error) {
	visited := make(map[string]bool)
	registry := sl.getBuilderRegistry()
	if registry != nil {
		if versions := registry.GetVersions(ontology); len(versions) > 0 {
			latestVersion, err := registry.GetLatestVersion(ontology)
			if err == nil {
				builder, err := registry.GetBuilder(ontology, latestVersion)
				if err == nil {
					resolved, resolveErr := sl.resolveSpecInheritance(builder.Build(), visited, depth)
					if resolveErr == nil {
						return resolved, nil
					}
				}
			}
		}
	}
	return sl.loadSpecWithInheritanceRecursive(ontology+".yaml", visited, "", depth)
}
