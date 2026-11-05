package swarm

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestSwarmSpawn_Hierarchy(t *testing.T) {
	t.Parallel()

	cmd := NewSwarmCmd()
	require.NotNil(t, cmd)

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["spawn"], "swarm command must have spawn subcommand")
}

func TestSwarmSpawn_RequiresTeamConfig(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	root := proj.Root
	t.Setenv("ZQK_PROJECT_ROOT", root)

	ctx := context.Background()
	store, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, root, store)

	cmd := NewSpawnCmd()
	cmd.SetArgs([]string{})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(t.Context(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "flag --team-config is required")
}

func TestSwarmSpawn_NeuronAndMuscleExecution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	root := proj.Root
	t.Setenv("ZQK_PROJECT_ROOT", root)

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, root, store)

	// Seed team_configuration for muscle cell
	muscleConfig := map[string]any{
		objects.FieldKeyID:     "TCFG-TEST-MUSCLE",
		objects.FieldKeyKind:   "team_configuration",
		objects.FieldKeyStatus: objects.ObjectStatusProposed,
		"cell_type":            "muscle",
		"focus_area":           "tactical",
		"persona_allocations": []any{
			map[string]any{
				"persona_ref": "PER-TEST-CRAFTSMAN",
				"role":        "craftsman_lead",
				"count":       1,
			},
		},
	}
	require.NoError(t, store.Create(ctx, sec, muscleConfig))

	// Seed priority plan
	planObj := map[string]any{
		objects.FieldKeyID:     "PRI-TEST-001",
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyTitle:  "Test Refactor Sprint",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	require.NoError(t, store.Create(ctx, sec, planObj))

	// Run swarm spawn
	cmd := NewSpawnCmd()
	cmd.SetArgs([]string{"--team-config", "TCFG-TEST-MUSCLE", "--plan", "PRI-TEST-001"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(t.Context(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err = cmd.Execute()
	require.NoError(t, err)

	got := out.String()
	assert.Contains(t, got, "SWARM SPAWN")
	assert.Contains(t, got, "Cellular Swarm Provisioned Successfully")
	assert.Contains(t, got, "TCFG-TEST-MUSCLE")
	assert.Contains(t, got, "muscle")
	assert.Contains(t, got, "tactical")
	assert.Contains(t, got, "PRI-TEST-001")
	assert.Contains(t, got, "PER-TEST-CRAFTSMAN")
}

func TestSwarmSpawn_JSONFormat(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SeedSchemaPlane: true,
	})
	root := proj.Root
	t.Setenv("ZQK_PROJECT_ROOT", root)

	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	store, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, root)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, root, store)

	neuronConfig := map[string]any{
		objects.FieldKeyID:     "TCFG-TEST-NEURON",
		objects.FieldKeyKind:   "team_configuration",
		objects.FieldKeyStatus: objects.ObjectStatusProposed,
		"cell_type":            "neuron",
		"focus_area":           "strategic",
	}
	require.NoError(t, store.Create(ctx, sec, neuronConfig))

	cmd := NewSpawnCmd()
	cmd.SetArgs([]string{"--team-config", "TCFG-TEST-NEURON", "-f", "json"})
	out := bytes.NewBuffer(nil)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(t.Context(), out))
	cmd.SetOut(out)
	cmd.SetErr(out)

	err = cmd.Execute()
	require.NoError(t, err)

	var payload map[string]any
	err = json.Unmarshal(out.Bytes(), &payload)
	require.NoError(t, err, "output should be valid JSON: %s", out.String())

	assert.Equal(t, "TCFG-TEST-NEURON", payload["team_config_id"])
	assert.Equal(t, "neuron", payload["cell_archetype"])
	assert.Equal(t, "strategic", payload["focus_area"])
	assert.NotEmpty(t, payload["pods"])
}
