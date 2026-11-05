package api_builders

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAPIGenerator_LifecycleAndGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	gen := NewAPIGenerator(tmpDir)
	require.NotNil(t, gen)

	// EnsureOutputDir
	err := gen.EnsureOutputDir()
	require.NoError(t, err)

	// Register dummy builder in registry
	reg := GetGlobalRegistry()
	bldr := &dummyAPISpecBuilder{
		fileName: "service_api",
		version:  "v1_0_0",
		data:     []byte("service: auth\n"),
	}
	reg.Register(bldr)

	// GenerateAPI specific version
	err = gen.GenerateAPI("service_api", "v1_0_0")
	require.NoError(t, err)

	content, err := fileutil.ReadFile(filepath.Join(tmpDir, "service_api.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "service: auth\n", string(content))

	// GenerateLatestAPI
	err = gen.GenerateLatestAPI("service_api")
	require.NoError(t, err)

	// GenerateAllAPIs
	err = gen.GenerateAllAPIs()
	require.NoError(t, err)

	// Error when builder does not exist
	err = gen.GenerateAPI("nonexistent_api", "v1_0_0")
	require.Error(t, err)

	err = gen.GenerateLatestAPI("nonexistent_api")
	require.Error(t, err)

	// Error when builder Build fails
	failBldr := &dummyAPISpecBuilder{
		fileName: "fail_api",
		version:  "v1_0_0",
		err:      errors.New("build failed"),
	}
	reg.Register(failBldr)
	err = gen.GenerateAPI("fail_api", "v1_0_0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build api")
}
