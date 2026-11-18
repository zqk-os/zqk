// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package community_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func registerTestDialect(t *testing.T, ex *community.FederatedExchange, name, ns, ver string, fields map[string]string) {
	t.Helper()
	err := ex.RegisterDialect(community.DialectSchema{
		Name:      name,
		Namespace: ns,
		Version:   ver,
		Fields:    fields,
	})
	if err != nil {
		t.Fatalf("failed to register dialect %s: %v", name, err)
	}
}

func registerTestRule(t *testing.T, ex *community.FederatedExchange, id, src, tgt string, mappings map[string]string) {
	t.Helper()
	err := ex.RegisterRule(community.TranslationRule{
		ID:            id,
		SourceDialect: src,
		TargetDialect: tgt,
		FieldMappings: mappings,
	})
	if err != nil {
		t.Fatalf("failed to register rule %s: %v", id, err)
	}
}

func assertRejectsBlankDialectFields(t *testing.T, ex *community.FederatedExchange) {
	t.Helper()
	if err := ex.RegisterDialect(community.DialectSchema{Name: "", Namespace: "ns", Version: "1.0"}); err == nil {
		t.Fatal("expected error on blank dialect name")
	}
	if err := ex.RegisterDialect(community.DialectSchema{Name: "a", Namespace: "", Version: "1.0"}); err == nil {
		t.Fatal("expected error on blank namespace")
	}
	if err := ex.RegisterDialect(community.DialectSchema{Name: "a", Namespace: "ns", Version: ""}); err == nil {
		t.Fatal("expected error on blank version")
	}
}

func TestFederatedExchange_StaticFloorInvariant(t *testing.T) {
	ex := community.NewFederatedExchange()

	registerTestDialect(t, ex, "dialect_a", "cluster-alpha", "1.0.0", map[string]string{
		"agent_id":    "string",
		"task_metric": "float64",
	})
	assertRejectsBlankDialectFields(t, ex)

	registerTestDialect(t, ex, "dialect_b", "cluster-beta", "2.0.0", map[string]string{
		"node_id":   "string",
		"score_val": "float64",
	})
	registerTestRule(t, ex, "rule-a-to-b", "dialect_a", "dialect_b", map[string]string{
		"agent_id":    "node_id",
		"task_metric": "score_val",
	})

	selfRule := community.TranslationRule{
		ID:            "rule-self",
		SourceDialect: "dialect_a",
		TargetDialect: "dialect_a",
		FieldMappings: map[string]string{"k": "v"},
	}
	if err := ex.RegisterRule(selfRule); err == nil {
		t.Fatal("expected error when source and target dialect are identical")
	}

	if err := ex.RegisterDialect(community.DialectSchema{
		Name:      "dialect_blank_field",
		Namespace: "cluster-gamma",
		Version:   "1.0.0",
		Fields:    map[string]string{" ": "string"},
	}); err == nil {
		t.Fatal("expected error on blank dialect schema field name")
	}

	if err := ex.RegisterRule(community.TranslationRule{
		ID:            "rule-blank-src-field",
		SourceDialect: "dialect_a",
		TargetDialect: "dialect_b",
		FieldMappings: map[string]string{" ": "node_id"},
	}); err == nil {
		t.Fatal("expected error on blank source field name in translation rule")
	}

	if err := ex.RegisterRule(community.TranslationRule{
		ID:            "rule-blank-tgt-field",
		SourceDialect: "dialect_a",
		TargetDialect: "dialect_b",
		FieldMappings: map[string]string{"agent_id": ""},
	}); err == nil {
		t.Fatal("expected error on blank target field name in translation rule")
	}

	if _, err := ex.TranslateDirect(community.FederatedQuery{
		ID:            "q-blank-field",
		SourceDialect: "dialect_a",
		TargetDialect: "dialect_b",
		Payload:       map[string]interface{}{" ": "val"},
	}); err == nil {
		t.Fatal("expected error on blank query payload field name")
	}
}

func setupChainExchange(t *testing.T) *community.FederatedExchange {
	t.Helper()
	ex := community.NewFederatedExchange()
	for _, d := range []string{"dialect_a", "dialect_b", "dialect_c"} {
		registerTestDialect(t, ex, d, fmt.Sprintf("ns-%s", d), "1.0.0", nil)
	}
	registerTestRule(t, ex, "rule-ab", "dialect_a", "dialect_b", map[string]string{
		"agent_name":   "worker_identity",
		"execution_ms": "elapsed_time",
	})
	registerTestRule(t, ex, "rule-bc", "dialect_b", "dialect_c", map[string]string{
		"worker_identity": "actor_urn",
		"elapsed_time":    "latency_duration",
	})
	return ex
}

func verifyConcurrentTranslations(t *testing.T, ex *community.FederatedExchange) {
	t.Helper()
	var wg sync.WaitGroup
	for w := 0; w < 10; w++ {
		wg.Add(1)
		workerID := w
		goroutinelabels.NewGoroutine("test_federated_exchange_worker", "concurrent translation testing").StartSimple(func() {
			defer wg.Done()
			q := community.FederatedQuery{
				ID:            fmt.Sprintf("q-conc-%d", workerID),
				SourceDialect: "dialect_a",
				TargetDialect: "dialect_b",
				Payload: map[string]interface{}{
					"agent_name":   fmt.Sprintf("worker-%d", workerID),
					"execution_ms": workerID * 10,
				},
			}
			res, err := ex.TranslateDirect(q)
			if err != nil {
				t.Errorf("concurrent translate error: %v", err)
				return
			}
			if res.TranslatedPayload["worker_identity"] != fmt.Sprintf("worker-%d", workerID) {
				t.Errorf("unexpected translated identity: %v", res.TranslatedPayload["worker_identity"])
			}
		})
	}
	wg.Wait()
}

func TestFederatedExchange_OperationalDynamicProof(t *testing.T) {
	ex := setupChainExchange(t)

	resAB, err := ex.TranslateDirect(community.FederatedQuery{
		ID:            "q-001",
		SourceDialect: "dialect_a",
		TargetDialect: "dialect_b",
		Payload:       map[string]interface{}{"agent_name": "holon-validator-01", "execution_ms": 42},
	})
	if err != nil {
		t.Fatalf("failed direct translation: %v", err)
	}
	if resAB.TranslatedPayload["worker_identity"] != "holon-validator-01" || resAB.TranslatedPayload["elapsed_time"] != 42 {
		t.Errorf("unexpected direct translation payload: %+v", resAB.TranslatedPayload)
	}

	resAC, err := ex.TranslateChain(community.FederatedQuery{
		ID:            "q-002",
		SourceDialect: "dialect_a",
		TargetDialect: "dialect_c",
		Payload:       map[string]interface{}{"agent_name": "holon-validator-02", "execution_ms": 100},
	}, []string{"dialect_b"})
	if err != nil {
		t.Fatalf("failed chain translation: %v", err)
	}
	if resAC.TranslatedPayload["actor_urn"] != "holon-validator-02" || resAC.TranslatedPayload["latency_duration"] != 100 {
		t.Errorf("unexpected chain translation payload: %+v", resAC.TranslatedPayload)
	}

	verifyConcurrentTranslations(t, ex)

	// Verify bidirectional field mapping and round-trip query conversion (100% field fidelity).
	biEx := community.NewFederatedExchange()
	for _, d := range []string{"dialect_a", "dialect_b", "dialect_c"} {
		registerTestDialect(t, biEx, d, fmt.Sprintf("ns-%s", d), "1.0.0", nil)
	}
	for _, r := range []community.TranslationRule{
		{
			ID:            "bi-ab",
			SourceDialect: "dialect_a",
			TargetDialect: "dialect_b",
			Bidirectional: true,
			FieldMappings: map[string]string{
				"agent_name":   "worker_identity",
				"execution_ms": "elapsed_time",
			},
		},
		{
			ID:            "bi-bc",
			SourceDialect: "dialect_b",
			TargetDialect: "dialect_c",
			Bidirectional: true,
			FieldMappings: map[string]string{
				"worker_identity": "actor_urn",
				"elapsed_time":    "latency_duration",
			},
		},
	} {
		if err := biEx.RegisterRule(r); err != nil {
			t.Fatalf("failed to register bidirectional rule %s: %v", r.ID, err)
		}
	}

	fwd, err := biEx.TranslateChain(community.FederatedQuery{
		ID:            "q-bi-fwd",
		SourceDialect: "dialect_a",
		TargetDialect: "dialect_c",
		Payload: map[string]interface{}{
			"agent_name":   "holon-validator-03",
			"execution_ms": 250,
			"trace_id":     "tr-999",
		},
	}, []string{"dialect_b"})
	if err != nil {
		t.Fatalf("failed forward bidirectional chain translation: %v", err)
	}
	if fwd.TranslatedPayload["actor_urn"] != "holon-validator-03" ||
		fwd.TranslatedPayload["latency_duration"] != 250 ||
		fwd.TranslatedPayload["trace_id"] != "tr-999" {
		t.Fatalf("unexpected forward bidirectional payload: %+v", fwd.TranslatedPayload)
	}

	rev, err := biEx.TranslateChain(community.FederatedQuery{
		ID:            "q-bi-rev",
		SourceDialect: "dialect_c",
		TargetDialect: "dialect_a",
		Payload:       fwd.TranslatedPayload,
	}, []string{"dialect_b"})
	if err != nil {
		t.Fatalf("failed reverse bidirectional chain translation: %v", err)
	}
	if rev.TranslatedPayload["agent_name"] != "holon-validator-03" ||
		rev.TranslatedPayload["execution_ms"] != 250 ||
		rev.TranslatedPayload["trace_id"] != "tr-999" {
		t.Fatalf("expected 100%% field fidelity on reverse translation, got: %+v", rev.TranslatedPayload)
	}
}

func assertCircularChainRejected(t *testing.T, ex *community.FederatedExchange) {
	t.Helper()
	registerTestRule(t, ex, "rule-xy", "dialect_x", "dialect_y", map[string]string{"a": "b"})
	registerTestRule(t, ex, "rule-yx", "dialect_y", "dialect_x", map[string]string{"b": "a"})

	queryCycle := community.FederatedQuery{
		ID:            "q-cycle",
		SourceDialect: "dialect_x",
		TargetDialect: "dialect_x",
		Payload:       map[string]interface{}{"a": 1},
	}
	if _, err := ex.TranslateChain(queryCycle, []string{"dialect_y"}); !errors.Is(err, community.ErrCircularTranslation) {
		t.Fatalf("expected ErrCircularTranslation on cyclic chain, got: %v", err)
	}
}

func TestFederatedExchange_NegativeAdversarialBoundary(t *testing.T) {
	ex := community.NewFederatedExchange()
	registerTestDialect(t, ex, "dialect_x", "ns", "1.0", nil)
	registerTestDialect(t, ex, "dialect_y", "ns", "1.0", nil)

	if err := ex.RegisterRule(community.TranslationRule{
		ID:            "bad-rule",
		SourceDialect: "unknown_dialect",
		TargetDialect: "dialect_y",
		FieldMappings: map[string]string{"k": "v"},
	}); !errors.Is(err, community.ErrUnsupportedDialect) {
		t.Fatalf("expected ErrUnsupportedDialect, got %v", err)
	}

	if _, err := ex.TranslateDirect(community.FederatedQuery{
		ID:            "q-bad",
		SourceDialect: "dialect_x",
		TargetDialect: "unknown_target",
		Payload:       map[string]interface{}{"k": "v"},
	}); !errors.Is(err, community.ErrUnsupportedDialect) {
		t.Fatalf("expected ErrUnsupportedDialect, got %v", err)
	}

	if _, err := ex.TranslateDirect(community.FederatedQuery{
		ID:            "q-no-rule",
		SourceDialect: "dialect_x",
		TargetDialect: "dialect_y",
		Payload:       map[string]interface{}{"k": "v"},
	}); !errors.Is(err, community.ErrTranslationRuleNotFound) {
		t.Fatalf("expected ErrTranslationRuleNotFound, got %v", err)
	}

	if err := ex.RegisterRule(community.TranslationRule{
		ID:            "colliding-bi-rule",
		SourceDialect: "dialect_x",
		TargetDialect: "dialect_y",
		Bidirectional: true,
		FieldMappings: map[string]string{"src_a": "shared_tgt", "src_b": "shared_tgt"},
	}); !errors.Is(err, community.ErrInvalidDialectSchema) {
		t.Fatalf("expected ErrInvalidDialectSchema on colliding bidirectional target fields, got %v", err)
	}

	assertCircularChainRejected(t, ex)
}
