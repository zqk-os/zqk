package paths

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAgentRuntimeFile_prefersNewDir(t *testing.T) {
	root := t.TempDir()
	neu := filepath.Join(root, AgentRuntimeRel(AgentChatChannelConfigFile))
	legacy := filepath.Join(root, ProjectDataDir, ConfigDir, AgentChatChannelConfigFile)
	if err := fileutil.EnsureDir(filepath.Dir(neu)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureDir(filepath.Dir(legacy)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(neu, []byte(`{"schema_version":"1"}`+"\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(legacy, []byte(`{"schema_version":"old"}`+"\n")); err != nil {
		t.Fatal(err)
	}
	got := AgentRuntimeFile(root, AgentChatChannelConfigFile)
	want, err := filepath.EvalSymlinks(neu)
	if err != nil {
		want = neu
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		gotResolved = got
	}
	if gotResolved != want {
		t.Fatalf("AgentRuntimeFile() = %q, want %q", got, neu)
	}
}

func TestAgentRuntimeFile_legacyFallbackAndWriteTarget(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ProjectDataDir, ConfigDir, AgentIdleStoreFile)
	if err := fileutil.EnsureDir(filepath.Dir(legacy)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(legacy, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	got := AgentRuntimeFile(root, AgentIdleStoreFile)
	want, err := filepath.EvalSymlinks(legacy)
	if err != nil {
		want = legacy
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		gotResolved = got
	}
	if gotResolved != want {
		t.Fatalf("legacy fallback = %q, want %q", got, legacy)
	}

	root2 := t.TempDir()
	got = AgentRuntimeFile(root2, AgentGitIdentityFile)
	want = filepath.Join(root2, AgentRuntimeRel(AgentGitIdentityFile))
	if got != want {
		t.Fatalf("write target = %q, want %q", got, want)
	}
}
