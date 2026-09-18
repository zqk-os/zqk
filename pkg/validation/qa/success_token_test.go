package qa

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestQASuccessTitle(t *testing.T) {
	got := QASuccessTitle("BLI-SUCCESS")
	if !strings.HasPrefix(got, qaSuccessTitlePrefix) {
		t.Fatalf("title %q missing prefix %q", got, qaSuccessTitlePrefix)
	}
	if !strings.Contains(got, "BLI-SUCCESS") {
		t.Fatalf("title %q missing item id", got)
	}
}

func TestBuildQASuccessObject_setsRequiredTitle(t *testing.T) {
	obj, err := buildQASuccessObject("BLI-TOKEN", "sighex", "pubkeyhex")
	if err != nil {
		t.Fatalf("buildQASuccessObject: %v", err)
	}
	title, _ := obj[objects.FieldKeyTitle].(string)
	if title != QASuccessTitle("BLI-TOKEN") {
		t.Fatalf("title=%q want %q", title, QASuccessTitle("BLI-TOKEN"))
	}
	if id, _ := obj[objects.FieldKeyID].(string); !strings.HasPrefix(id, "QAS-") {
		t.Fatalf("id=%q want QAS- prefix", id)
	}
	if kind, _ := obj[objects.FieldKeyKind].(string); kind != KindQASuccess {
		t.Fatalf("kind=%q want %q", kind, KindQASuccess)
	}
	if item, _ := obj[objects.FieldKeyItemID].(string); item != "BLI-TOKEN" {
		t.Fatalf("item_id=%q want BLI-TOKEN", item)
	}
	if st, _ := obj[objects.FieldKeyStatus].(string); st != objects.ObjectStatusSuccess {
		t.Fatalf("status=%q want %q", st, objects.ObjectStatusSuccess)
	}
}
