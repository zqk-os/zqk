package vds

import (
	"context"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// Durable glossary identity (portable across projects). Instance GLS-* CAS ids are not.
const (
	GlossaryCanonicalTitle = "Verifiable Decomposition Spine (VDS)"
	GlossaryAcronymTitle   = "VDS"
	GlossaryCLITitle       = "CLI Command: workflow vds"
	KindGlossaryTerm       = "glossary_term"
)

// GlossaryBinding is optional project-local pinning of GLS-* ids (customization only).
// Prefer resolving by title when pins are empty so new kernels do not require Go rebuilds.
type GlossaryBinding struct {
	TermRef    string `yaml:"term_ref" json:"term_ref,omitempty"`
	AcronymRef string `yaml:"acronym_ref" json:"acronym_ref,omitempty"`
	CLIRef     string `yaml:"cli_ref" json:"cli_ref,omitempty"`
}

// GlossaryRefs are resolved ids for the current project kernel (may be empty).
type GlossaryRefs struct {
	TermRef    string `json:"term_ref,omitempty"`
	AcronymRef string `json:"acronym_ref,omitempty"`
	CLIRef     string `json:"cli_ref,omitempty"`
	// ResolvedBy: customization | title_lookup | mixed | unresolved
	ResolvedBy string `json:"resolved_by,omitempty"`
}

// TitleLookup finds an object id by kind + exact title (optional).
type TitleLookup func(ctx context.Context, kind, title string) (id string, err error)

// ResolveGlossary resolves VDS glossary refs without hardcoding CAS ids in Go.
// Order per field: customization pin → title lookup → leave empty.
func ResolveGlossary(ctx context.Context, cust *Customization, lookup TitleLookup) GlossaryRefs {
	var pins GlossaryBinding
	if cust != nil {
		pins = cust.Glossary
	}
	out := GlossaryRefs{}
	usedPin := false
	usedLookup := false

	resolve := func(pin, title string) string {
		if id := strings.TrimSpace(pin); id != "" {
			usedPin = true
			return id
		}
		if lookup == nil || strings.TrimSpace(title) == "" {
			return ""
		}
		id, err := lookup(ctx, KindGlossaryTerm, title)
		if err != nil || strings.TrimSpace(id) == "" {
			return ""
		}
		usedLookup = true
		return strings.TrimSpace(id)
	}

	out.TermRef = resolve(pins.TermRef, GlossaryCanonicalTitle)
	out.AcronymRef = resolve(pins.AcronymRef, GlossaryAcronymTitle)
	out.CLIRef = resolve(pins.CLIRef, GlossaryCLITitle)

	switch {
	case usedPin && usedLookup:
		out.ResolvedBy = "mixed"
	case usedPin:
		out.ResolvedBy = "customization"
	case usedLookup:
		out.ResolvedBy = "title_lookup"
	default:
		out.ResolvedBy = "unresolved"
	}
	return out
}

// CultureGlossaryLine is a portable culture sentence (titles + policy; ids only if resolved).
func CultureGlossaryLine(refs GlossaryRefs) string {
	base := "Glossary: " + GlossaryCanonicalTitle + " (acronym " + GlossaryAcronymTitle + "); policy " + PolicyID
	if refs.TermRef != "" {
		base += "; term_ref=" + refs.TermRef
		if refs.AcronymRef != "" {
			base += "; acronym_ref=" + refs.AcronymRef
		}
	} else {
		base += "; resolve via title or customization.glossary.* (GLS ids are project-local)"
	}
	return base
}

// IDFromObject extracts objects.FieldKeyID from a map.
func IDFromObject(obj map[string]any) string {
	if obj == nil {
		return ""
	}
	if id, ok := obj[objects.FieldKeyID].(string); ok {
		return strings.TrimSpace(id)
	}
	return ""
}
