package agent

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestSeatWorkerLane_DirectEnv(t *testing.T) {
	t.Setenv(zqkenv.WorkerLane().Name(), "frontend-lane")
	t.Setenv(zqkenv.SeatWorkerLane().Name(), "")

	lane := seatWorkerLane("PER-TEST", "agent-1")
	assert.Equal(t, "frontend-lane", lane)

	t.Setenv(zqkenv.WorkerLane().Name(), "")
	t.Setenv(zqkenv.SeatWorkerLane().Name(), "backend-lane")
	lane = seatWorkerLane("PER-TEST", "agent-1")
	assert.Equal(t, "backend-lane", lane)
}

func TestSeatWorkerLane_EnvMapped(t *testing.T) {
	t.Setenv(zqkenv.WorkerLane().Name(), "")
	t.Setenv(zqkenv.SeatWorkerLane().Name(), "")

	// Test JSON format
	t.Setenv(zqkenv.WorkerLanes().Name(), `{"PER-ORCH-DEV": "dev-lane", "agent-special": "special-lane"}`)
	assert.Equal(t, "dev-lane", seatWorkerLane("PER-ORCH-DEV", "agent-1"))
	assert.Equal(t, "special-lane", seatWorkerLane("OTHER", "agent-special"))

	// Test comma-separated format
	t.Setenv(zqkenv.WorkerLanes().Name(), "PER-ALPHA=alpha-lane,agent-9=nine-lane")
	assert.Equal(t, "alpha-lane", seatWorkerLane("per-alpha", ""))
	assert.Equal(t, "nine-lane", seatWorkerLane("", "agent-9"))
}

func TestSeatWorkerLane_PersonaDerivation(t *testing.T) {
	t.Setenv(zqkenv.WorkerLane().Name(), "")
	t.Setenv(zqkenv.SeatWorkerLane().Name(), "")
	t.Setenv(zqkenv.WorkerLanes().Name(), "")

	tmpDir := t.TempDir()

	assert.Equal(t, "swarm-coordinator", seatWorkerLaneWithRoot(tmpDir, "PER-ORCH-SWARM-COORDINATOR", ""))
	assert.Equal(t, "release-manager", seatWorkerLaneWithRoot(tmpDir, "ORCH-RELEASE-MANAGER", ""))
}

func TestSeatWorkerLane_HashFallbackAndSanitize(t *testing.T) {
	t.Setenv(zqkenv.WorkerLane().Name(), "")
	t.Setenv(zqkenv.SeatWorkerLane().Name(), "")
	t.Setenv(zqkenv.WorkerLanes().Name(), "")

	tmpDir := t.TempDir()

	// Deterministic agent ID hash
	lane := seatWorkerLaneWithRoot(tmpDir, "", "seat-worker-12345")
	assert.Len(t, lane, 8)

	// Completely empty returns unknown lane
	assert.Equal(t, seatWorkerLaneUnknown, seatWorkerLaneWithRoot(tmpDir, "", ""))

	// Engine ID format
	engineID := seatWorkerEngineIDWithRoot(tmpDir, "PER-ORCH-LEAD", "agent-x")
	assert.NotEmpty(t, engineID)
	assert.Contains(t, engineID, "seat-worker-lead-")

	engineIDDefault := seatWorkerEngineID("PER-ORCH-LEAD", "agent-x")
	assert.NotEmpty(t, engineIDDefault)
}

func TestSeatWorkerLane_ConfigFileResolution(t *testing.T) {
	t.Setenv(zqkenv.WorkerLane().Name(), "")
	t.Setenv(zqkenv.SeatWorkerLane().Name(), "")
	t.Setenv(zqkenv.WorkerLanes().Name(), "")

	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, paths.ConfigDir)
	require.NoError(t, fileutil.EnsureDir(cfgDir))

	// YAML config
	cfgPathYAML := filepath.Join(cfgDir, "worker_lanes.yaml")
	cfgContentYAML := `
default_lane: "default-config-lane"
seats:
  seat-qa-1: "qa-testing"
personas:
  PER-DEVELOPER: "dev-feature"
`
	require.NoError(t, fileutil.WriteStandardFile(cfgPathYAML, []byte(cfgContentYAML)))

	assert.Equal(t, "qa-testing", seatWorkerLaneWithRoot(tmpDir, "", "seat-qa-1"))
	assert.Equal(t, "dev-feature", seatWorkerLaneWithRoot(tmpDir, "PER-DEVELOPER", ""))
	assert.Equal(t, "default-config-lane", seatWorkerLaneWithRoot(tmpDir, "", ""))
}

func TestSeatWorkerLane_JSONConfigFileResolution(t *testing.T) {
	t.Setenv(zqkenv.WorkerLane().Name(), "")
	t.Setenv(zqkenv.SeatWorkerLane().Name(), "")
	t.Setenv(zqkenv.WorkerLanes().Name(), "")

	tmpDir := t.TempDir()
	rtDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.AgentRuntimeDir)
	require.NoError(t, fileutil.EnsureDir(rtDir))

	cfgPathJSON := filepath.Join(rtDir, "worker_lanes.json")
	cfgContentJSON := `{
		"default_lane": "json-default",
		"seats": {"seat-json": "lane-json-seat"},
		"personas": {"PER-JSON": "lane-json-persona"}
	}`
	require.NoError(t, fileutil.WriteStandardFile(cfgPathJSON, []byte(cfgContentJSON)))

	assert.Equal(t, "lane-json-seat", seatWorkerLaneWithRoot(tmpDir, "", "seat-json"))
	assert.Equal(t, "lane-json-persona", seatWorkerLaneWithRoot(tmpDir, "PER-JSON", ""))
	assert.Equal(t, "json-default", seatWorkerLaneWithRoot(tmpDir, "", ""))
}
