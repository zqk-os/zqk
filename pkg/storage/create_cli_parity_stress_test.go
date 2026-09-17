package storage

import (
	"fmt"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestCreateCLIParityStress_RealProjectRoot reproduces CLI create→get flakes against this checkout.
// Skip unless ZQK_STRESS_REAL_ROOT=1 (destructive to local drafts).
func TestCreateCLIParityStress_RealProjectRoot(t *testing.T) {
	if zqkenv.StressRealRoot().Get() != "1" {
		t.Skip("set " + zqkenv.StressRealRoot().Name() + "=1 to run against checkout root")
	}
	// From pkg/storage, module root is ../..
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	if _, err := fileutil.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("go.mod not found from %s: %v", root, err)
	}
	ctx := pkgctx.NewSystemContext()
	sec := pkgctx.NewSystemSecurityContext()
	factory, err := NewStorageFactory(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	sp := factory.GetStorage()
	t.Logf("storage type=%T root=%s", sp, root)

	ok, fail := 0, 0
	for i := 0; i < 25; i++ {
		obj := map[string]any{
			objects.FieldKeyKind:  objects.KindBacklogItem,
			objects.FieldKeyTitle: fmt.Sprintf("stress-inproc-%d", i),
		}
		cliCtx := WithCLIOperation(WithSkipWriteBehind(ctx))
		if err := sp.Create(cliCtx, sec, obj); err != nil {
			fail++
			t.Logf("Create err i=%d: %v keys=%v", i, err, keysOf(obj))
			continue
		}
		id, _ := obj[objects.FieldKeyID].(string)
		st, _ := obj[objects.FieldKeyStatus].(string)
		_, readErr := sp.Read(cliCtx, sec, id)
		draft := ObjectDraftPlanePath(root, objects.KindBacklogItem, id)
		_, statErr := fileutil.Stat(draft)
		if readErr != nil || statErr != nil {
			fail++
			t.Logf("FAIL i=%d id=%s status=%q keys=%v read=%v draftStat=%v", i, id, st, keysOf(obj), readErr, statErr)
			continue
		}
		ok++
	}
	t.Logf("SUMMARY ok=%d fail=%d", ok, fail)
	if fail > 0 {
		t.Fatalf("create→get failures: %d/%d", fail, ok+fail)
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
