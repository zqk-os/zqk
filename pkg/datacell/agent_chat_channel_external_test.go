package datacell_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReadAgentChatChannelConfig_validFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfgDir := filepath.Join(root, paths.ProjectDataDir, paths.AgentRuntimeDir)
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(cfgDir, paths.AgentChatChannelConfigFile)
	payload, err := json.Marshal(map[string]any{
		objects.FieldKeySchemaVersion: "1",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyNote:          "pilot",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(p, payload); err != nil {
		t.Fatal(err)
	}
	c, err := datacell.ReadAgentChatChannelConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Enabled || c.Note != "pilot" {
		t.Fatalf("got %#v", c)
	}
}

func TestReadAgentChatChannelConfig_deliveryMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfgDir := filepath.Join(root, paths.ProjectDataDir, paths.AgentRuntimeDir)
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(cfgDir, paths.AgentChatChannelConfigFile)
	payload, err := json.Marshal(map[string]any{
		objects.FieldKeySchemaVersion: "1",
		objects.FieldKeyEnabled:       true,
		objects.FieldKeyDeliveryMode:  "notify",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(p, payload); err != nil {
		t.Fatal(err)
	}
	c, err := datacell.ReadAgentChatChannelConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.DeliveryMode != "notify" {
		t.Fatalf("delivery_mode: %#v", c)
	}
}
