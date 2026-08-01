package storage

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/stretchr/testify/assert"
)

func TestSemanticCompressionConfig_Policy(t *testing.T) {
	root := t.TempDir()
	policyDir := filepath.Join(root, paths.ProjectDataDir, "..", "docs", "process", "compression_policy")
	if err := fileutil.EnsureDir(policyDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	policyData := []byte(`
kind: compression_policy
id: COMP-1
target_kind: test_event
compress_field_keys: true
zlib_threshold_bytes: 1024
`)
	if err := fileutil.WriteSecureFile(filepath.Join(policyDir, "test.yaml"), policyData); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Clear cache for test
	semanticCompressionMu.Lock()
	compressionPolicies = make(map[string]*CompressionPolicy)
	semanticCompressionMu.Unlock()

	policy := GetCompressionPolicy(root, "test_event")
	if policy == nil {
		t.Fatal("expected policy to be loaded")
	}

	if !policy.CompressFieldKeys {
		t.Errorf("expected CompressFieldKeys true, got false")
	}
	if policy.ZlibThresholdBytes != 1024 {
		t.Errorf("expected ZlibThresholdBytes 1024, got %d", policy.ZlibThresholdBytes)
	}
}

func TestGetDefaultCompressionPolicyForKind(t *testing.T) {
	kinds := []string{"graph_node", "memory_node", "audit_event", "scheduler_job"}
	for _, kind := range kinds {
		policy := GetDefaultCompressionPolicyForKind(kind)
		assert.NotNil(t, policy)
		assert.Equal(t, "zlib", policy.Algorithm)
		assert.True(t, policy.OmitSchemaDefaults)
		assert.True(t, policy.CompressFieldKeys)
		assert.Equal(t, 512, policy.ThresholdBytes)
		assert.Equal(t, 1024, policy.ZlibThresholdBytes)
	}

	defaultPolicy := GetDefaultCompressionPolicyForKind("generic_kind")
	assert.NotNil(t, defaultPolicy)
	assert.Equal(t, "none", defaultPolicy.Algorithm)
	assert.False(t, defaultPolicy.OmitSchemaDefaults)
}
