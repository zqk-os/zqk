package vds

import (
	"context"
	"strings"
	"testing"
)

func TestResolveGlossary_customizationPins(t *testing.T) {
	cust := &Customization{Glossary: GlossaryBinding{
		TermRef:    "GLS-PIN-TERM",
		AcronymRef: "GLS-PIN-ACRO",
	}}
	got := ResolveGlossary(context.Background(), cust, nil)
	if got.TermRef != "GLS-PIN-TERM" || got.AcronymRef != "GLS-PIN-ACRO" {
		t.Fatalf("pins: %+v", got)
	}
	if got.ResolvedBy != "customization" {
		t.Fatalf("resolved_by=%s", got.ResolvedBy)
	}
}

func TestResolveGlossary_titleLookup(t *testing.T) {
	lookup := func(_ context.Context, kind, title string) (string, error) {
		if kind != KindGlossaryTerm {
			t.Fatalf("kind=%s", kind)
		}
		switch title {
		case GlossaryCanonicalTitle:
			return "GLS-FROM-TITLE", nil
		case GlossaryAcronymTitle:
			return "GLS-ACRO-TITLE", nil
		default:
			return "", nil
		}
	}
	got := ResolveGlossary(context.Background(), &Customization{}, lookup)
	if got.TermRef != "GLS-FROM-TITLE" || got.AcronymRef != "GLS-ACRO-TITLE" {
		t.Fatalf("lookup: %+v", got)
	}
	if got.ResolvedBy != "title_lookup" {
		t.Fatalf("resolved_by=%s", got.ResolvedBy)
	}
}

func TestResolveGlossary_pinBeatsTitle(t *testing.T) {
	lookup := func(_ context.Context, _, _ string) (string, error) {
		return "GLS-SHOULD-NOT-WIN", nil
	}
	cust := &Customization{Glossary: GlossaryBinding{TermRef: "GLS-PIN"}}
	got := ResolveGlossary(context.Background(), cust, lookup)
	if got.TermRef != "GLS-PIN" {
		t.Fatalf("pin should win: %+v", got)
	}
}

func TestCultureGlossaryLine_unresolved(t *testing.T) {
	s := CultureGlossaryLine(GlossaryRefs{ResolvedBy: "unresolved"})
	if !strings.Contains(s, PolicyID) || !strings.Contains(s, GlossaryCanonicalTitle) {
		t.Fatalf("line=%q", s)
	}
	if !strings.Contains(s, "project-local") {
		t.Fatalf("expected unresolved guidance: %q", s)
	}
}
