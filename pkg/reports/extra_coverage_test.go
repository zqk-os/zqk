// BLI-STARTER-COMMUNITY-031 / PRI-STARTER-COMMUNITY-031 coverage elevation
package reports

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestGenerateReports_SuccessPayloads(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	mat, err := GenerateMaturationReport(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := mat.(map[string]any)
	if !ok || m["report_type"] != "maturation" || m[objects.FieldKeyStatus] != "success" {
		t.Fatalf("%v", mat)
	}
	vit, err := GenerateVitalityReport(ctx, map[string]any{"unused": true})
	if err != nil {
		t.Fatal(err)
	}
	v := vit.(map[string]any)
	if v["report_type"] != "vitality" {
		t.Fatalf("%v", vit)
	}
	qa, err := GenerateQASuccessReport(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	q := qa.(map[string]any)
	if q["report_type"] != "qa_success" {
		t.Fatalf("%v", qa)
	}
}
