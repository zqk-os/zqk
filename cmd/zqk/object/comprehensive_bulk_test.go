package object

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestAllKindsBulkOperations tests bulk operations for all discoverable object kinds
func TestAllKindsBulkOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("comprehensive all-kinds bulk ops is slow; run without -short")
	}
	fieldRegistry := objects.GetGlobalFieldRegistry()
	if err := fieldRegistry.LoadFields(); err != nil {
		t.Fatalf("failed to load field registry: %v", err)
	}

	kinds, err := fieldRegistry.GetAllKinds()
	if err != nil {
		t.Fatalf("failed to get all kinds: %v", err)
	}
	if raceDetectorEnabled {
		kinds = []string{pplanKindBacklogItem, "goal", "system_check"}
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	tmpDir, cliBinary := setupCLITestEnvironmentForComprehensive(t)

	RunComprehensiveKindTests(t, kinds, func(t *testing.T, kind string) {
		if kind == "zqk_session" {
			t.Skip("zqk_session uses stream_current + subprocess bulk; full bulk ops suite is deferred until stream/CLI apply is aligned in temp roots")
		}
		if kind == "mcp_session" {
			t.Skip("mcp_session is stream-backed; bulk create/read verification via in-proc storage in this suite races subprocess WAL/stream apply — defer until stream bulk path is aligned with zqk_session work")
		}
		// TRACK: BLI-COMMS-CURSOR-TPM-DELIVER-ATTN-001 adjacent — remove when harness seeds org refs.
		if kind == "partnership" {
			t.Skip("partnership bulk test is currently failing to seed reference organization cross-kind dependency in temp harness.")
		}
		testKindBulkOperations(t, cliBinary, tmpDir, ctx, secCtx, kind, fieldRegistry)
	})
}

// testKindBulkOperations tests bulk operations for a specific kind
func testKindBulkOperations(t *testing.T, cliBinary, tmpDir string, ctx context.Context, secCtx *pkgctx.SecurityContext, kind string, fieldRegistry *objects.FieldRegistry) {
	kindFields, err := fieldRegistry.GetFieldsForKind(kind)
	if err != nil {
		t.Skipf("skipping %s: failed to get fields: %v", kind, err)
		return
	}

	t.Run("BulkCreate", func(t *testing.T) {
		testBulkCreateForKind(t, cliBinary, kind, kindFields, tmpDir)
	})

	t.Run("BulkGet", func(t *testing.T) {
		testBulkGetForKind(t, cliBinary, kind, tmpDir)
	})

	t.Run("BulkUpdate", func(t *testing.T) {
		testBulkUpdateForKind(t, cliBinary, tmpDir, ctx, secCtx, kind, kindFields)
	})

	t.Run("BulkDelete", func(t *testing.T) {
		testBulkDeleteForKind(t, cliBinary, tmpDir, kind)
	})
}
