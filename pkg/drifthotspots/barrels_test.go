package drifthotspots

import "testing"

func TestIsConstantBarrelPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"pkg/foo/lock_op_names.go", true},
		{"pkg/specbuilder/bldr_v2/backlog_item_constants.go", true},
		{"pkg/specbuilder/bldr_v2/nested/x_constants.go", true},
		{"cmd/zqk/system/check.go", false},
	}
	for _, tc := range cases {
		if got := IsConstantBarrelPath(tc.path); got != tc.want {
			t.Errorf("IsConstantBarrelPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsDataCatalogPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"pkg/storage/object_storage_dynamic_test_helper.go", true},
		{"pkg/specbuilder/bldr_config_v1/kind_mappings_config_builder.go", true},
		{"pkg/objects/kind_mappings.go", true},
		{"scripts/generate-readme-index.go", true},
		{"cmd/zqk/system/check.go", false},
	}
	for _, tc := range cases {
		if got := IsDataCatalogPath(tc.path); got != tc.want {
			t.Errorf("IsDataCatalogPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestBuildReport_FileSummaries(t *testing.T) {
	opts := Options{RootDir: ".", SpecsDir: ".", MinRisk: "low"}
	findings := []Finding{
		{File: "a.go", Line: 1, Column: 1, Risk: RiskCritical, Category: CategoryKindLiteralCompare, Literal: "x"},
		{File: "a.go", Line: 2, Column: 1, Risk: RiskMedium, Category: CategoryKindLiteralAssign, Literal: "y"},
		{File: "b.go", Line: 1, Column: 1, Risk: RiskMedium, Category: CategorySystemFieldKey, Literal: "kind"},
	}
	r := BuildReport(opts, findings)
	if len(r.FileSummaries) != 2 {
		t.Fatalf("file summaries: %d", len(r.FileSummaries))
	}
	if r.FileSummaries[0].File != "a.go" || r.FileSummaries[0].Count != 2 {
		t.Fatalf("first summary: %+v", r.FileSummaries[0])
	}
	if r.CountByRisk[RiskCritical] != 1 || r.CountByCategory[CategoryKindLiteralCompare] != 1 {
		t.Fatalf("counts: %#v %#v", r.CountByRisk, r.CountByCategory)
	}
}
