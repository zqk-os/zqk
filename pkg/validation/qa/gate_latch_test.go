package qa

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	testLatchBacklogItemID    = "BLI-LATCH-ALL-PASS"
	testValidGoContent        = "package valid\n"
	testCleanFileName         = "valid_clean.go"
	testErrSignerFailed       = "NewAuditorSigner failed"
	testErrGatePassedExpected = "expected VerifyComplete to pass all 3 latches"
)

func TestAuditorGate_LatchCountdownSequenceDedicated(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	tmpDir := t.TempDir()

	store := &mockGateStore{
		objs: make(map[string]map[string]any),
	}

	signer, err := NewAuditorSigner("")
	require.NoError(t, err, testErrSignerFailed)

	cleanFile := filepath.Join(tmpDir, testCleanFileName)
	err = os.WriteFile(cleanFile, []byte(testValidGoContent), 0644)
	require.NoError(t, err)

	store.objs[testLatchBacklogItemID] = map[string]any{
		objects.FieldKeyID:        testLatchBacklogItemID,
		objects.FieldKeyKind:      objects.KindBacklogItem,
		objects.FieldKeyArtifacts: []any{cleanFile},
	}

	status := objects.ObjectStatusSuccess
	data := []byte(testLatchBacklogItemID + status)
	sig, err := signer.Sign(data)
	require.NoError(t, err)

	qaObj, err := buildQASuccessObject(testLatchBacklogItemID, sig, signer.PublicKey())
	require.NoError(t, err)
	qaObj[objects.FieldKeyStatus] = status

	err = store.Create(ctx, secCtx, qaObj)
	require.NoError(t, err)

	err = gateVerify(ctx, store, testLatchBacklogItemID)
	require.NoError(t, err, testErrGatePassedExpected)
}

func gateVerify(ctx context.Context, store *mockGateStore, itemID string) error {
	gate := NewAuditorGate(store)
	return gate.VerifyComplete(ctx, itemID)
}
