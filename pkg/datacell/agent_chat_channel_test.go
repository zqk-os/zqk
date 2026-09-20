package datacell

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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
	wantDir := filepath.Join(root, paths.ProjectDataDir, paths.LogsDir, paths.IDEHooksLogsSubdir)
	if st, err := fileutil.Stat(wantDir); err != nil || !st.IsDir() {
		t.Fatalf("dir: %v err=%v", st, err)
	}
}
