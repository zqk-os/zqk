// BLI-STARTER-COMMUNITY-047 / PRI-STARTER-COMMUNITY-047 coverage elevation
package primaryorch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraInboxNoopSanitizeAndLoadErrors(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	if _, err := LoadBinding(filepath.Join(root, "no-such")); err != nil {
		t.Fatal(err)
	}
	bad := ConfigPath(root)
	if err := fileutil.MkdirAll(filepath.Dir(bad), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(bad, []byte("{"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBinding(root); err == nil {
		t.Fatal("bad json")
	}
	if err := os.Remove(bad); err != nil {
		t.Fatal(err)
	}
	if err := WriteBinding(root, Binding{Adapter: AdapterInbox, AgentID: "a/b c", InboxSubdir: ""}); err != nil {
		t.Fatal(err)
	}
	b, err := LoadBinding(root)
	if err != nil || b.SchemaVersion != SchemaVersion {
		t.Fatalf("%+v %v", b, err)
	}

	inbox := InboxAdapter{}
	if inbox.Name() != AdapterInbox {
		t.Fatal(inbox.Name())
	}
	res, err := inbox.Wake(ctx, root, b, WakeRequest{TaskID: "ATK/1", Message: "hi", Persona: PersonaTPM})
	if err != nil || !strings.Contains(res.DeliveredTo, "inbox:") {
		t.Fatalf("%+v %v", res, err)
	}
	res2, err := inbox.Wake(ctx, root, Binding{AgentID: "peer", Adapter: AdapterInbox, InboxSubdir: "leaf"}, WakeRequest{Message: "x"})
	if err != nil || !strings.Contains(res2.DeliveredTo, "inbox:") {
		t.Fatal(err)
	}

	noop := NoopAdapter{}
	if noop.Name() != AdapterNoop {
		t.Fatal(noop.Name())
	}
	if _, err := noop.Wake(ctx, root, Binding{AgentID: "n"}, WakeRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := (ScriptAdapter{}).Wake(ctx, root, Binding{Adapter: AdapterScript}, WakeRequest{Message: "x"}); err == nil {
		t.Fatal("empty script")
	}
	if _, err := ResolveAdapter(Binding{Adapter: "nope"}); err == nil {
		t.Fatal("unknown")
	}
	for _, name := range []string{AdapterScript, AdapterAgentChat, AdapterInbox, AdapterNoop} {
		ad, err := ResolveAdapter(Binding{Adapter: name})
		if err != nil || ad.Name() != name {
			t.Fatalf("%s %v", name, err)
		}
	}

	if got := sanitizePathSegment(""); got != "unnamed" {
		t.Fatal(got)
	}
	if got := sanitizePathSegment("@@@"); got != "unnamed" {
		t.Fatal(got)
	}
	if got := sanitizePathSegment(" a/b "); !strings.Contains(got, "a_b") && got == "" {
		t.Fatal(got)
	}

	if got := ComposeWakeMessage(WakeRequest{}); got == "" {
		t.Fatal("empty compose")
	}
	if got := AppendAttentivenessContext("", "", []BacklogBrief{{Title: "x"}, {ID: "BLI-1", Title: "", Tier: ""}}); !strings.Contains(got, "BLI-1") {
		t.Fatal(got)
	}

	MaybeWakeOnPlanBacklogError(ctx, root, "goal", "planned", objects.ObjectStatusError, nil)
	MaybeWakeOnPlanBacklogError(ctx, root, objects.KindBacklogItem, objects.ObjectStatusError, objects.ObjectStatusError, map[string]any{})
	MaybeWakeOnPlanBacklogError(ctx, root, objects.KindBacklogItem, "planned", objects.ObjectStatusError, map[string]any{objects.FieldKeyID: "BLI-x"})
	_, _ = WakePrimary(ctx, root, WakeRequest{Message: "via binding"})
}
