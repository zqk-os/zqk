package authcred

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPrototypeAccountError(t *testing.T) {
	err := PrototypeAccountError
	if err.Error() != "prototype/test account" {
		t.Fatalf("unexpected error message: %s", err.Error())
	}
	if !errors.Is(err, PrototypeAccountError) {
		t.Fatal("expected errors.Is to match PrototypeAccountError")
	}
	if errors.Is(err, errors.New("other")) {
		t.Fatal("expected errors.Is not to match random error")
	}
}

func TestPrototypeAccountRegistry(t *testing.T) {
	const customRef = "acc-custom-test-ref-123"
	RegisterPrototypeAccountRefs(customRef)
	if !isRegisteredPrototypeRef(customRef) {
		t.Fatalf("expected %q to be registered", customRef)
	}

	ResetPrototypeAccountRefs()
	if isRegisteredPrototypeRef(customRef) {
		t.Fatalf("expected %q to be cleared", customRef)
	}

	ResetPrototypeAccountRefs(defaultPrototypeAccountRefs...)
}

func TestKindAllowedInDiscovery(t *testing.T) {
	if KindAllowedInDiscovery(DiscoveryLaneFull, "") {
		t.Fatal("empty kind should not be allowed")
	}
	if !KindAllowedInDiscovery(DiscoveryLaneFull, "anything") {
		t.Fatal("full lane should allow any kind")
	}
	if !KindAllowedInDiscovery(DiscoveryLanePlanner, PlannerDiscoveryKinds[0]) {
		t.Fatal("planner lane should allow planner kind")
	}
	if KindAllowedInDiscovery(DiscoveryLanePlanner, "non_existent_kind_xyz") {
		t.Fatal("planner lane should not allow random kind")
	}
	if !KindAllowedInDiscovery(DiscoveryLaneDoer, DoerDiscoveryKinds[0]) {
		t.Fatal("doer lane should allow doer kind")
	}
	if KindAllowedInDiscovery(DiscoveryLaneDoer, "non_existent_kind_xyz") {
		t.Fatal("doer lane should not allow random kind")
	}
	if !KindAllowedInDiscovery(DiscoveryLane("custom_unknown_lane"), "any") {
		t.Fatal("default lane should allow kind")
	}
}

func TestInvalidateKeystore(t *testing.T) {
	root := t.TempDir()
	InvalidateKeystore(root)
	InvalidateKeystore("")
}

func TestTokenFingerprintMeta(t *testing.T) {
	meta := TokenFingerprintMeta("key-1", "hash-123", "2026-10-01T00:00:00Z")
	if meta[objects.FieldKeyTokenID] != "key-1" {
		t.Fatalf("unexpected token id: %v", meta)
	}
	if meta[tokenMetaExpiration] != "2026-10-01T00:00:00Z" {
		t.Fatalf("unexpected expiration: %v", meta)
	}

	metaNoExp := TokenFingerprintMeta("key-2", "hash-456", "")
	if _, ok := metaNoExp[tokenMetaExpiration]; ok {
		t.Fatalf("expected no expiration in meta: %v", metaNoExp)
	}
}

func TestAccountYAMLAndWalk(t *testing.T) {
	root := t.TempDir()
	dir := paths.AccountsDirPath(root)
	if err := fileutil.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	const accID = "ACC-TEST-USER-001"
	const hash = "1111222233334444"
	body := "id: " + accID + "\nkind: account\n"
	if err := fileutil.WriteStandardFile(paths.AccountYAMLPath(root, hash), []byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(paths.AccountIndexPath(root), []byte(`{"mappings":{"`+accID+`":"`+hash+`","OTHER-123":"`+hash+`"}}`)); err != nil {
		t.Fatal(err)
	}

	raw, ok := AccountYAML(root, accID)
	if !ok || string(raw) != body {
		t.Fatalf("AccountYAML got ok=%v, body=%q", ok, string(raw))
	}
	if _, ok := AccountYAML("", accID); ok {
		t.Fatal("AccountYAML with empty root should fail")
	}
	if _, ok := AccountYAML(root, ""); ok {
		t.Fatal("AccountYAML with empty id should fail")
	}

	var foundIDs []string
	WalkAccountYAML(root, func(id string, data []byte) bool {
		foundIDs = append(foundIDs, id)
		return true
	})
	if len(foundIDs) != 1 || foundIDs[0] != accID {
		t.Fatalf("WalkAccountYAML found: %v", foundIDs)
	}

	WalkAccountYAML("", func(id string, data []byte) bool { return true })
	WalkAccountYAML(root, nil)
}

func TestRequireRBACSpecs(t *testing.T) {
	if err := RequireRBACSpecs(""); err == nil {
		t.Fatal("expected error for empty project root")
	}

	root := t.TempDir()
	specsDir := paths.ObjectSpecsDir(root)
	if err := fileutil.EnsureDir(specsDir); err != nil {
		t.Fatal(err)
	}

	// Missing specs should fail
	if err := RequireRBACSpecs(root); err == nil {
		t.Fatal("expected error when account/role specs are missing")
	}

	// Create account spec
	if err := os.WriteFile(filepath.Join(specsDir, "account.yaml"), []byte("kind: account\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := RequireRBACSpecs(root); err == nil {
		t.Fatal("expected error when role spec is missing")
	}

	// Create role spec
	if err := os.WriteFile(filepath.Join(specsDir, "role.yaml"), []byte("kind: role\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := RequireRBACSpecs(root); err != nil {
		t.Fatalf("unexpected error when both specs exist: %v", err)
	}
}
