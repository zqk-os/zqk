// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package kindsynonyms

import (
	"fmt"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/kindnames"
)

func TestDefaultKindAliasesForKind_FoundAndNotFound(t *testing.T) {
	t.Parallel()

	// Verify alias retrieval for known kinds
	entries := DefaultKindAliasEntries()
	if len(entries) == 0 {
		t.Fatalf("expected DefaultKindAliasEntries to return non-empty slice")
	}

	for _, entry := range entries {
		aliases := DefaultKindAliasesForKind(entry.Kind)
		if len(aliases) != len(entry.Aliases) {
			t.Fatalf("mismatch in alias count for %s: got %d, expected %d", entry.Kind, len(aliases), len(entry.Aliases))
		}
	}

	// Verify unknown kind returns an empty slice (covers the not-found branch)
	unknownAliases := DefaultKindAliasesForKind("non_existent_kind_xyz")
	if len(unknownAliases) != 0 {
		t.Fatalf("expected empty slice for unknown kind, got %v", unknownAliases)
	}
}

func TestNormalizeKind(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"   ", ""},
		{"  GOAL  ", "goal"},
		{"\tRequirement\n", "requirement"},
		{"BACKLOG_ITEM", "backlog_item"},
	}

	for _, tc := range cases {
		normalized := NormalizeKind(tc.input)
		if normalized != tc.expected {
			t.Fatalf("NormalizeKind(%q) = %q, expected %q", tc.input, normalized, tc.expected)
		}
	}
}

func TestCanonicalKindResolution_DefaultMappedKinds(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	canonicalKinds := []string{
		kindnames.Goal,
		kindnames.Requirement,
		kindnames.Criteria,
		kindnames.TestCase,
		kindnames.BacklogItem,
		kindnames.Epic,
		kindnames.Milestone,
		kindnames.ChangeJournalEntry,
		kindnames.Policy,
		kindnames.PromptTemplate,
		kindnames.VocabularyScheme,
		kindnames.Component,
		kindnames.Decision,
		kindnames.PriorityPlan,
		kindnames.Workstream,
		kindnames.StrategicPlan,
		kindnames.WorkstreamTransition,
		kindnames.EvolutionManagement,
	}

	for _, k := range canonicalKinds {
		resolved := resolver.ResolveKind(k)
		if resolved != k {
			t.Fatalf("expected canonical kind %q to resolve to itself, got %q", k, resolved)
		}
		if !resolver.IsCanonical(k) {
			t.Fatalf("expected IsCanonical(%q) to be true", k)
		}
	}
}

func TestSynonymAndAbbreviationResolution(t *testing.T) {
	t.Parallel()

	cases := []struct {
		alias        string
		expectedKind string
	}{
		// Abbreviations & plurals
		{"req", kindnames.Requirement},
		{"reqs", kindnames.Requirement},
		{"crit", kindnames.Criteria},
		{"crits", kindnames.Criteria},
		{"bli", kindnames.BacklogItem},
		{"blis", kindnames.BacklogItem},
		{"pri", kindnames.PriorityPlan},
		{"pris", kindnames.PriorityPlan},
		{"pplan", kindnames.PriorityPlan},
		{"pplans", kindnames.PriorityPlan},
		{"goal", kindnames.Goal},
		{"goals", kindnames.Goal},
		{"tst", kindnames.TestCase},
		{"tsts", kindnames.TestCase},
		{"tc", kindnames.TestCase},
		{"tcs", kindnames.TestCase},
		{"epic", kindnames.Epic},
		{"epics", kindnames.Epic},
		{"mil", kindnames.Milestone},
		{"mils", kindnames.Milestone},
		{"journal", kindnames.ChangeJournalEntry},
		{"policies", kindnames.Policy},
		{"vocab", kindnames.VocabularyScheme},
		{"task", kindnames.BacklogItem},
		{"tasks", kindnames.BacklogItem},
		{"adr", kindnames.Decision},
		{"comp", kindnames.Component},
		{"prompt", kindnames.PromptTemplate},
		{"splan", kindnames.StrategicPlan},
		{"wstrans", kindnames.WorkstreamTransition},
		{"evoman", kindnames.EvolutionManagement},
	}

	for _, tc := range cases {
		resolved := ResolveKind(tc.alias)
		if resolved != tc.expectedKind {
			t.Fatalf("ResolveKind(%q) = %q, expected %q", tc.alias, resolved, tc.expectedKind)
		}
	}
}

func TestResolveKind_CasingAndWhitespaceNormalization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw      string
		expected string
	}{
		{"  REQ  ", kindnames.Requirement},
		{"\tBLI\n", kindnames.BacklogItem},
		{"  CRITS  ", kindnames.Criteria},
		{"  priority_plan  ", kindnames.PriorityPlan},
		{"", ""},
		{"   ", ""},
	}

	for _, tc := range cases {
		resolved := ResolveKind(tc.raw)
		if resolved != tc.expected {
			t.Fatalf("ResolveKind(%q) = %q, expected %q", tc.raw, resolved, tc.expected)
		}
	}
}

func TestResolveKind_PluralNormalizationFallback(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()

	// Register custom kind and alias without plural entries to exercise dynamic plural suffix resolution
	if err := resolver.Register("cluster", "node"); err != nil {
		t.Fatalf("unexpected error registering cluster/node: %v", err)
	}

	// Plural of canonical kind ("clusters" -> "cluster")
	resCluster := resolver.ResolveKind("clusters")
	if resCluster != "cluster" {
		t.Fatalf("expected 'clusters' to resolve to 'cluster', got %q", resCluster)
	}

	// Plural of alias ("nodes" -> "node" -> "cluster")
	resNode := resolver.ResolveKind("nodes")
	if resNode != "cluster" {
		t.Fatalf("expected 'nodes' to resolve to 'cluster', got %q", resNode)
	}

	// "components" is an alias or plural of "component"
	resolved := resolver.ResolveKind("components")
	if resolved != kindnames.Component {
		t.Fatalf("expected 'components' to resolve to %q, got %q", kindnames.Component, resolved)
	}

	// Unknown kind with plural suffix should fallback pass-through
	unknownPlural := "widgets"
	resUnknown := resolver.ResolveKind(unknownPlural)
	if resUnknown != unknownPlural {
		t.Fatalf("expected unknown plural to pass through %q, got %q", unknownPlural, resUnknown)
	}
}

func TestResolveKindStrict(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()

	// Empty string returns error
	emptyRes, errEmpty := resolver.ResolveKindStrict("")
	if errEmpty == nil {
		t.Fatalf("expected error for empty kind string, got %q", emptyRes)
	}

	// Whitespace only returns error
	spaceRes, errSpace := resolver.ResolveKindStrict("   ")
	if errSpace == nil {
		t.Fatalf("expected error for whitespace kind string, got %q", spaceRes)
	}

	// Unknown kind returns error
	unkRes, errUnk := resolver.ResolveKindStrict("unregistered_mystery_kind")
	if errUnk == nil {
		t.Fatalf("expected error for unknown kind, got %q", unkRes)
	}

	// Valid kind returns resolved canonical kind
	validRes, errValid := resolver.ResolveKindStrict("  req  ")
	if errValid != nil {
		t.Fatalf("expected success for 'req', got error: %v", errValid)
	}
	if validRes != kindnames.Requirement {
		t.Fatalf("expected 'req' to resolve to %q, got %q", kindnames.Requirement, validRes)
	}

	// Package-level ResolveKindStrict
	pkgRes, errPkg := ResolveKindStrict("bli")
	if errPkg != nil {
		t.Fatalf("expected package ResolveKindStrict to succeed, got error: %v", errPkg)
	}
	if pkgRes != kindnames.BacklogItem {
		t.Fatalf("expected 'bli' to resolve to %q, got %q", kindnames.BacklogItem, pkgRes)
	}
}

func TestCustomSynonymRegistration(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()

	// Empty canonical kind
	errEmptyKind := resolver.Register("", "svc")
	if errEmptyKind == nil {
		t.Fatalf("expected error when registering empty canonical kind")
	}

	// Empty synonym
	errEmptySynonym := resolver.Register("service", "")
	if errEmptySynonym == nil {
		t.Fatalf("expected error when registering empty synonym")
	}

	// Synonym identical to canonical kind
	errSame := resolver.Register("service", "service")
	if errSame != nil {
		t.Fatalf("expected nil when synonym matches canonical kind, got: %v", errSame)
	}

	// Valid registration
	errReg := resolver.Register("service", "svc")
	if errReg != nil {
		t.Fatalf("expected successful registration of custom synonym, got: %v", errReg)
	}

	if resolved := resolver.ResolveKind("svc"); resolved != "service" {
		t.Fatalf("expected 'svc' to resolve to 'service', got %q", resolved)
	}

	// Registering same synonym for same kind again (idempotent)
	errIdempotent := resolver.Register("service", "svc")
	if errIdempotent != nil {
		t.Fatalf("expected idempotent re-registration to succeed, got: %v", errIdempotent)
	}

	// Conflicting registration to another kind
	errConflict := resolver.Register("cluster", "svc")
	if errConflict == nil {
		t.Fatalf("expected error when registering conflicting synonym for different kind")
	}
}

func TestAliasesForKindAndAllCanonicalKinds(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()

	// Aliases for known kind
	reqAliases := resolver.AliasesForKind(kindnames.Requirement)
	if len(reqAliases) == 0 {
		t.Fatalf("expected non-empty aliases for requirement")
	}

	foundReq := false
	for _, a := range reqAliases {
		if a == "req" {
			foundReq = true
			break
		}
	}
	if !foundReq {
		t.Fatalf("expected 'req' in requirement aliases: %v", reqAliases)
	}

	// Aliases for unknown kind
	unknownAliases := resolver.AliasesForKind("unknown_random_kind")
	if len(unknownAliases) != 0 {
		t.Fatalf("expected empty aliases for unknown kind, got %v", unknownAliases)
	}

	// AllCanonicalKinds
	allKinds := resolver.AllCanonicalKinds()
	if len(allKinds) == 0 {
		t.Fatalf("expected non-empty AllCanonicalKinds")
	}

	// Check IsCanonical for empty
	if resolver.IsCanonical("") {
		t.Fatalf("expected IsCanonical('') to be false")
	}
}

func TestConcurrentResolverOperations(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	var wg sync.WaitGroup
	workers := 16
	iterations := 50

	for i := 0; i < workers; i++ {
		workerID := i
		wg.Add(1)
		goroutinelabels.NewGoroutine("kindsynonyms_test", fmt.Sprintf("worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Query canonical
				res := resolver.ResolveKind("req")
				if res != kindnames.Requirement {
					panic(fmt.Sprintf("worker %d: unexpected resolution %q", workerID, res))
				}

				// Query abbreviation
				resBli := resolver.ResolveKind("bli")
				if resBli != kindnames.BacklogItem {
					panic(fmt.Sprintf("worker %d: unexpected bli resolution %q", workerID, resBli))
				}

				// Check canonical status
				if !resolver.IsCanonical(kindnames.Goal) {
					panic(fmt.Sprintf("worker %d: expected goal to be canonical", workerID))
				}

				// Check aliases
				aliases := resolver.AliasesForKind(kindnames.Criteria)
				if len(aliases) == 0 {
					panic(fmt.Sprintf("worker %d: expected criteria aliases", workerID))
				}
			}
		})
	}

	wg.Wait()
}
