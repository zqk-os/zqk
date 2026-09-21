package validation

import (
	"maps"
	"slices"
)

func cloneStringSliceMap(in map[string][]string) map[string][]string {
	if in == nil {
		return nil
	}
	out := make(map[string][]string, len(in))
	for k, v := range in {
		out[k] = slices.Clone(v)
	}
	return out
}

func cloneIDPrefixesConfig(in *IDPrefixesConfig) *IDPrefixesConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.KindToPrefixes = cloneStringSliceMap(in.KindToPrefixes)
	out.KindToSynonyms = cloneStringSliceMap(in.KindToSynonyms)
	out.InferenceRules.Patterns = slices.Clone(in.InferenceRules.Patterns)
	if n := len(in.InferenceRules.SynonymPatterns); n > 0 {
		pats := make([]SynonymInferencePattern, n)
		for i, p := range in.InferenceRules.SynonymPatterns {
			p.Strategies = slices.Clone(p.Strategies)
			pats[i] = p
		}
		out.InferenceRules.SynonymPatterns = pats
	} else {
		out.InferenceRules.SynonymPatterns = slices.Clone(in.InferenceRules.SynonymPatterns)
	}
	return &out
}

func cloneNamespacesConfig(in *NamespacesConfig) *NamespacesConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.NamespaceLayers = slices.Clone(in.NamespaceLayers)
	out.InferenceRules = slices.Clone(in.InferenceRules)
	if in.Namespaces != nil {
		out.Namespaces = make(map[string]NamespaceConfig, len(in.Namespaces))
		for k, ns := range in.Namespaces {
			ns.Kinds = slices.Clone(ns.Kinds)
			ns.SubordinateNamespaces = slices.Clone(ns.SubordinateNamespaces)
			out.Namespaces[k] = ns
		}
	}
	return &out
}

func clonePathsConfig(in *PathsConfig) *PathsConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.Paths = maps.Clone(in.Paths)
	out.SearchStrategy.RelativePaths = slices.Clone(in.SearchStrategy.RelativePaths)
	out.SearchStrategy.Markers = slices.Clone(in.SearchStrategy.Markers)
	return &out
}

func cloneValidationTimeoutConfig(in *ValidationTimeoutConfig) *ValidationTimeoutConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.KindOverrides = maps.Clone(in.KindOverrides)
	return &out
}

func cloneValidationTierConfig(in *ValidationTierConfig) *ValidationTierConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.BlockingTiers = slices.Clone(in.BlockingTiers)
	out.RuleToTierMapping = maps.Clone(in.RuleToTierMapping)
	return &out
}
