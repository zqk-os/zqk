package api_builders

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummyAPISpecBuilder struct {
	fileName string
	version  string
	data     []byte
	err      error
}

func (d *dummyAPISpecBuilder) Build() ([]byte, error) {
	return d.data, d.err
}

func (d *dummyAPISpecBuilder) GetVersion() string {
	return d.version
}

func (d *dummyAPISpecBuilder) GetFileName() string {
	return d.fileName
}

func TestVersionedAPISpecGenBuilderRegistry(t *testing.T) {
	r := NewVersionedAPISpecGenBuilderRegistry()
	require.NotNil(t, r)

	// Query non-existent
	_, err := r.GetBuilder("unknown", "v1_0_0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no builders found")

	_, err = r.GetLatestVersion("unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no builders found")

	assert.Nil(t, r.GetVersions("unknown"))

	// Register builders
	b1 := &dummyAPISpecBuilder{fileName: "api1", version: "v1_0_0", data: []byte("v1")}
	b2 := &dummyAPISpecBuilder{fileName: "api1", version: "v1_2_0", data: []byte("v1_2")}
	b3 := &dummyAPISpecBuilder{fileName: "api1", version: "v1_1_5", data: []byte("v1_1_5")}
	b4 := &dummyAPISpecBuilder{fileName: "api2", version: "custom_ver", data: []byte("custom")}

	r.Register(b1)
	r.Register(b2)
	r.Register(b3)
	r.Register(b4)

	// GetBuilder
	got, err := r.GetBuilder("api1", "v1_2_0")
	require.NoError(t, err)
	assert.Equal(t, b2, got)

	// GetBuilder version mismatch
	_, err = r.GetBuilder("api1", "v9_9_9")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no builder found for api_spec")

	// GetLatestVersion semantic sorting
	latest, err := r.GetLatestVersion("api1")
	require.NoError(t, err)
	assert.Equal(t, "v1_2_0", latest)

	// GetLatestVersion unparseable fallback
	customLatest, err := r.GetLatestVersion("api2")
	require.NoError(t, err)
	assert.Equal(t, "custom_ver", customLatest)

	// GetAllFileNames
	names := r.GetAllFileNames()
	assert.Contains(t, names, "api1")
	assert.Contains(t, names, "api2")

	// GetVersions
	vers := r.GetVersions("api1")
	assert.Len(t, vers, 3)
	assert.Contains(t, vers, "v1_0_0")
	assert.Contains(t, vers, "v1_2_0")
	assert.Contains(t, vers, "v1_1_5")
}

func TestGlobalRegistry(t *testing.T) {
	reg := GetGlobalRegistry()
	require.NotNil(t, reg)

	builder := &dummyAPISpecBuilder{fileName: "global_test", version: "v1_0_0"}
	RegisterBuilder(builder)

	got, err := reg.GetBuilder("global_test", "v1_0_0")
	require.NoError(t, err)
	assert.Equal(t, builder, got)
}
