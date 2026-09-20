package inbox

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockStorage struct {
	storage.ObjectStorageProvider
	objects map[string]map[string]any
}

func (m *mockStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	id := "AGI-1234"
	obj[objects.FieldKeyID] = id
	m.objects[id] = obj
	return nil
}

func (m *mockStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return m.objects[id], nil
}

func TestIngestProposal(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	store := &mockStorage{
		objects: make(map[string]map[string]any),
	}

	t.Run("success", func(t *testing.T) {
		params := IngestionParams{
			Instruction:   "Fix the hyperdrive",
			TargetPersona: "agent:mechanic",
			SourceSession: "cvs-123",
			CreatedBy:     "account:han",
		}

		obj, err := IngestProposal(ctx, secCtx, store, params)
		require.NoError(t, err)
		assert.NotNil(t, obj)

		// Verify fields
		assert.Equal(t, "agent_instruction", obj[objects.FieldKeyKind])
		assert.Equal(t, "proposed", obj[objects.FieldKeyStatus])
		assert.Equal(t, "Fix the hyperdrive", obj[objects.FieldKeyInstruction])
		assert.Equal(t, "agent:mechanic", obj[objects.FieldKeyTargetPersona])
		assert.Equal(t, "cvs-123", obj[objects.FieldKeySourceSession])
		assert.Equal(t, "account:han", obj[objects.FieldKeyCreatedBy])

		// Verify persistence
		id, _ := obj[objects.FieldKeyID].(string)
		fetched, err := store.Read(ctx, secCtx, id)
		require.NoError(t, err)
		assert.Equal(t, obj[objects.FieldKeyInstruction], fetched[objects.FieldKeyInstruction])
	})

	t.Run("empty_instruction_fails", func(t *testing.T) {
		params := IngestionParams{
			Instruction: "",
		}

		obj, err := IngestProposal(ctx, secCtx, store, params)
		assert.Error(t, err)
		assert.Nil(t, obj)
		assert.Contains(t, err.Error(), "instruction cannot be empty")
	})
}
