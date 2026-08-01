package datacell

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestReadAgentChatChannelConfig_missingFile_defaults(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	c, err := ReadAgentChatChannelConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Enabled {
		t.Fatal("expected disabled by default")
	}
	if c.SchemaVersion != AgentChatChannelSchemaVersion {
		t.Fatalf("schema: %q", c.SchemaVersion)
	}
}

func TestEnsureAgentChatChannelEventsDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := EnsureAgentChatChannelEventsDir(root); err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, paths.CursorHooksLogsSubdir)
	if st, err := os.Stat(wantDir); err != nil || !st.IsDir() {
		t.Fatalf("dir: %v err=%v", st, err)
	}
}
