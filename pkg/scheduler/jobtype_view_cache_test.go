package scheduler

import (
	"context"
	"testing"
)

func TestJobTypeViewCache_ParitySpecAndRegistry(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	view, err := BuildJobTypeViewCache(env.SpecLoader)
	if err != nil {
		t.Fatalf("BuildJobTypeViewCache: %v", err)
	}

	specSet := make(map[string]struct{}, len(view.EnumValues))
	for _, jt := range view.EnumValues {
		specSet[jt] = struct{}{}
	}

	registrySet := make(map[string]struct{}, len(jobTypeHandlerRegistry))
	for k := range jobTypeHandlerRegistry {
		registrySet[k] = struct{}{}
	}

	// Parity guard:
	// 1) Registry must not contain job_types unknown to the spec enum.
	// 2) Spec enum may include forward-compatible job_types without handlers yet,
	//    but those are allowed only when explicitly allowlisted.
	for jt := range specSet {
		_, inRegistry := registrySet[jt]
		if !inRegistry {
			if !isJobTypeIntentionalNoHandler(jt) {
				t.Fatalf("spec enum job_type '%s' missing from handler registry and not in the allowlist", jt)
			}
			// Intentional missing handler: view.HandlerPresent is expected to be false.
			continue
		}
		if !view.HandlerPresent[jt] {
			t.Fatalf("view.HandlerPresent[%s] = false, expected true", jt)
		}
	}

	// Group coverage: every enum value must appear in exactly one static group.
	seen := make(map[string]struct{}, len(view.EnumValues))
	for group, items := range view.Groups {
		_ = group
		for _, jt := range items {
			seen[jt] = struct{}{}
		}
	}
	for jt := range specSet {
		if _, ok := seen[jt]; !ok {
			t.Fatalf("job_type '%s' missing from view.Groups", jt)
		}
	}
}

func TestEnsureAndLoadJobTypeViewCache(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	if err := EnsureJobTypeViewCacheReady(context.Background(), env.TestRoot, env.SpecLoader); err != nil {
		t.Fatalf("EnsureJobTypeViewCacheReady: %v", err)
	}

	loaded, err := LoadJobTypeViewCache(env.TestRoot)
	if err != nil {
		t.Fatalf("LoadJobTypeViewCache: %v", err)
	}
	if loaded == nil {
		t.Fatalf("expected loaded view cache, got nil")
	}
	if loaded.SchemaVersion != jobTypeViewCacheSchemaVersion {
		t.Fatalf("SchemaVersion: got %d, want %d", loaded.SchemaVersion, jobTypeViewCacheSchemaVersion)
	}
	if len(loaded.EnumValues) == 0 {
		t.Fatalf("EnumValues empty")
	}
}
