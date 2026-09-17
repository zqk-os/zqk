package systemcheckwake

import "testing"

func TestEvaluate_defaults(t *testing.T) {
	cfg := DefaultConfig()
	if reasons := Evaluate(Summary{}, cfg); len(reasons) != 0 {
		t.Fatalf("empty summary should not trip: %v", reasons)
	}
	reasons := Evaluate(Summary{DraftPlaneTotal: 1}, cfg)
	if len(reasons) != 1 || reasons[0].Name != "draft_plane" {
		t.Fatalf("got %#v", reasons)
	}
	reasons = Evaluate(Summary{
		ErrorStatusObjects: 2,
		BlockingIssues:     3,
		Warnings:           1,
		Informational:      1,
	}, cfg)
	if len(reasons) != 4 {
		t.Fatalf("want 4 reasons, got %#v", reasons)
	}
}

func TestEvaluate_recommendations_opt_in(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IncludeRecommendations = false
	if reasons := Evaluate(Summary{Recommendations: 5}, cfg); len(reasons) != 0 {
		t.Fatalf("recommendations should be off: %v", reasons)
	}
	cfg.IncludeRecommendations = true
	one := 1
	cfg.MinRecommendations = &one
	reasons := Evaluate(Summary{Recommendations: 5}, cfg)
	if len(reasons) != 1 || reasons[0].Name != "recommendations" {
		t.Fatalf("got %#v", reasons)
	}
}

func TestEvaluate_min_zero_disables(t *testing.T) {
	cfg := DefaultConfig()
	zero := 0
	cfg.MinWarnings = &zero
	reasons := Evaluate(Summary{Warnings: 99, BlockingIssues: 1}, cfg)
	if len(reasons) != 1 || reasons[0].Name != "blocking" {
		t.Fatalf("got %#v", reasons)
	}
}

func TestResolveNotifyAgent_sentinel(t *testing.T) {
	root := t.TempDir()
	// no binding → DefaultBinding (env or "primary")
	id, err := ResolveNotifyAgent(root, PrimarySentinel)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("empty agent id")
	}
	id2, err := ResolveNotifyAgent(root, "antigravity-1")
	if err != nil {
		t.Fatal(err)
	}
	if id2 != "antigravity-1" {
		t.Fatalf("got %q", id2)
	}
}

func TestLoadConfig_missing(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if derefMin(cfg.MinBlocking, -1) != 1 {
		t.Fatalf("default min blocking: %v", cfg.MinBlocking)
	}
}
