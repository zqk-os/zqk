package paths

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAgentRuntimeFile_usesAgentRuntimeOnly(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, AgentRuntimeRel(AgentChatChannelConfigFile))
	leftover := filepath.Join(root, ProjectDataDir, ConfigDir, AgentChatChannelConfigFile)
	if err := fileutil.EnsureDir(filepath.Dir(leftover)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(leftover, []byte(`{"schema_version":"old"}`+"\n")); err != nil {
		t.Fatal(err)
	}
	got := AgentRuntimeFile(root, AgentChatChannelConfigFile)
	if got != want {
		t.Fatalf("AgentRuntimeFile() = %q, want %q (leftover .zqk/config must not be read)", got, want)
	}
}

func TestAgentRuntimeFile_writeTarget(t *testing.T) {
	root := t.TempDir()
	got := AgentRuntimeFile(root, AgentGitIdentityFile)
	want := filepath.Join(root, AgentRuntimeRel(AgentGitIdentityFile))
	if got != want {
		t.Fatalf("write target = %q, want %q", got, want)
	}
}
