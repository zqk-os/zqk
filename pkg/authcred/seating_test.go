package authcred

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestHashAndResolveSecret(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, paths.ProcessKeystoreDir)
	if err := fileutil.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	secret := SecretPrefix + "test-secret-001"
	hash := HashAPIKey(secret)
	entry := `account_id: ACC-SEAT-001
credential_hash: ` + hash + `
id: KEY-TEST-001
key_type: api_key
kind: keystore_entry
revoked: false
status: active
`
	if err := fileutil.WriteSecureFile(filepath.Join(dir, "deadbeef.yaml"), []byte(entry)); err != nil {
		t.Fatal(err)
	}

	m, err := ResolveSecret(root, secret)
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if m.AccountID != "ACC-SEAT-001" || m.KeyID != "KEY-TEST-001" {
		t.Fatalf("match=%+v", m)
	}
	again, err := ResolveSecret(root, secret)
	if err != nil || again.AccountID != "ACC-SEAT-001" {
		t.Fatalf("cached ResolveSecret=%+v err=%v", again, err)
	}

	if _, err := ResolveSecret(root, "wrong"); err == nil {
		t.Fatal("expected miss")
	}
}

func TestSeatCredentialRoundTrip(t *testing.T) {
	root := t.TempDir()
	const acc = "ACC-SEAT-002"
	secret := SecretPrefix + "seat"
	if err := WriteSeatCredential(root, acc, secret); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSeatCredential(root, acc)
	if err != nil || got != secret {
		t.Fatalf("got %q err=%v", got, err)
	}
	if APIKeyForSeat(root, acc) != secret {
		t.Fatalf("APIKeyForSeat want secret")
	}
	if APIKeyForSeat(root, "ACC-MISSING") != "ACC-MISSING" {
		t.Fatalf("fallback to ACC id")
	}

	// Verify file permissions (0600)
	path := SeatCredentialPath(root, acc)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode() & 0777; mode != 0600 {
		t.Fatalf("expected permissions 0600, got %v", mode)
	}
}

func TestResolveSeatAccount(t *testing.T) {
	root := t.TempDir()
	accDir := filepath.Join(root, paths.ProcessDir, "accounts")
	if err := fileutil.EnsureDir(accDir); err != nil {
		t.Fatal(err)
	}
	const (
		accID     = "ACC-SEAT-PER-001"
		personaID = "PER-SEAT-001"
		hashName  = "aabbccdd00112233445566778899aabbccddeeff00112233445566778899aabb"
	)
	idx := `{"mappings":{"` + accID + `":"` + hashName + `"}}`
	if err := fileutil.WriteStandardFile(filepath.Join(accDir, ".account.index"), []byte(idx)); err != nil {
		t.Fatal(err)
	}
	yamlBody := "id: " + accID + "\nstatus: active\npersona_ref: " + personaID + "\nroles:\n  - swarm_worker\n"
	if err := fileutil.WriteStandardFile(filepath.Join(accDir, hashName+".yaml"), []byte(yamlBody)); err != nil {
		t.Fatal(err)
	}

	if got := ResolveSeatAccount(root, accID); got != accID {
		t.Fatalf("ACC passthrough: got %q", got)
	}
	if got := ResolveSeatAccount(root, personaID); got != accID {
		t.Fatalf("persona map: got %q want %q", got, accID)
	}
	if got := ResolveSeatAccount(root, "swarm_worker"); got != accID {
		t.Fatalf("role map: got %q want %q", got, accID)
	}
	if got := ResolveSeatAccount(root, "PER-UNKNOWN"); got != DefaultSwarmWorkerAccount {
		t.Fatalf("fallback: got %q", got)
	}
	if got := ResolveSeatAccount(root, "PER-ORCH-MISSING"); got != DefaultSwarmWorkerAccount {
		t.Fatalf("unbound persona is not planner just because id contains orch: got %q", got)
	}
	if got := ResolveSeatAccount(root, "cap_orchestrator"); got != DefaultSwarmWorkerAccount {
		t.Fatalf("unknown role label without a planner role object is not a planner miss: got %q", got)
	}
}

func TestResolveSeatAccount_plannerFromRoleObject(t *testing.T) {
	root := t.TempDir()
	accDir := filepath.Join(root, paths.ProcessDir, "accounts")
	roleDir := filepath.Join(root, paths.ProcessRolesDir)
	if err := fileutil.EnsureDir(accDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureDir(roleDir); err != nil {
		t.Fatal(err)
	}
	const (
		accID    = "ACC-PLAN-001"
		accHash  = "ccddeeff00112233445566778899aabbccddeeff00112233445566778899aabb"
		roleHash = "ddeeff00112233445566778899aabbccddeeff00112233445566778899aabbcc"
	)
	if err := fileutil.WriteStandardFile(filepath.Join(accDir, ".account.index"), []byte(`{"mappings":{"`+accID+`":"`+accHash+`"}}`)); err != nil {
		t.Fatal(err)
	}
	accYAML := "id: " + accID + "\nstatus: active\nusername: planner\npersona_ref: PER-PLAN-001\nroles:\n  - cap_orchestrator\n"
	if err := fileutil.WriteStandardFile(filepath.Join(accDir, accHash+".yaml"), []byte(accYAML)); err != nil {
		t.Fatal(err)
	}
	roleYAML := "id: ROL-AGENT-ORCH\nstatus: active\nrole_id: agent-cap-orchestrator\naliases:\n  - cap_orchestrator\npermissions:\n  - agent:orchestrate\n"
	if err := fileutil.WriteStandardFile(filepath.Join(roleDir, roleHash+".yaml"), []byte(roleYAML)); err != nil {
		t.Fatal(err)
	}

	if got := ResolveSeatAccount(root, "agent-cap-orchestrator"); got != accID {
		t.Fatalf("planner role_id should seat planner ACC: got %q", got)
	}
	if got := ResolveSeatAccount(root, "PER-PLAN-001"); got != accID {
		t.Fatalf("planner persona: got %q", got)
	}
	if got := ResolveSeatAccount(root, "ROL-MISSING-PLANNER"); got != DefaultSwarmWorkerAccount {
		t.Fatalf("unknown ref is doer fallback: got %q", got)
	}

	// Planner role exists, no account carries it — fail closed (empty).
	root2 := t.TempDir()
	if err := fileutil.EnsureDir(filepath.Join(root2, paths.ProcessRolesDir)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(filepath.Join(root2, paths.ProcessRolesDir, "role.yaml"), []byte(roleYAML)); err != nil {
		t.Fatal(err)
	}
	if got := ResolveSeatAccount(root2, "cap_orchestrator"); got != "" {
		t.Fatalf("planner role with no seated ACC must miss, got %q", got)
	}
}

func TestCanonicalAccountID(t *testing.T) {
	if got := CanonicalAccountID("", "account:swarm_worker"); got != "" {
		t.Fatalf("unresolved without project root: got %q want empty", got)
	}
	if got := CanonicalAccountID("", DefaultSwarmWorkerAccount); got != DefaultSwarmWorkerAccount {
		t.Fatalf("ACC passthrough: got %q", got)
	}
	root := t.TempDir()
	accDir := filepath.Join(root, paths.ProcessDir, "accounts")
	if err := fileutil.EnsureDir(accDir); err != nil {
		t.Fatal(err)
	}
	const (
		accID    = "ACC-USER-LOOKUP-001"
		hashName = "bbccddee00112233445566778899aabbccddeeff00112233445566778899aabb"
	)
	idx := `{"mappings":{"` + accID + `":"` + hashName + `"}}`
	if err := fileutil.WriteStandardFile(filepath.Join(accDir, ".account.index"), []byte(idx)); err != nil {
		t.Fatal(err)
	}
	yamlBody := "id: " + accID + "\nstatus: active\nusername: custom_swarm\n"
	if err := fileutil.WriteStandardFile(filepath.Join(accDir, hashName+".yaml"), []byte(yamlBody)); err != nil {
		t.Fatal(err)
	}
	if got := CanonicalAccountID(root, "account:custom_swarm"); got != accID {
		t.Fatalf("username index: got %q want %q", got, accID)
	}
	if path := AccountCASPath(root, accID); path == "" || !strings.HasSuffix(path, hashName+".yaml") {
		t.Fatalf("AccountCASPath: %q", path)
	}
}

func TestWithSeatAPIKeyEnv(t *testing.T) {
	parent := []string{"FOO=1", "ZQK_API_KEY=ACC-PARENT", "ZQK_PROJECT_ROOT=/old"}
	out := WithSeatAPIKeyEnv(parent, SecretPrefix+"x", "/wt")
	if !containsEnv(out, "ZQK_API_KEY="+SecretPrefix+"x") {
		t.Fatalf("missing seat key: %v", out)
	}
	if containsEnv(out, "ZQK_API_KEY=ACC-PARENT") {
		t.Fatalf("parent key leaked: %v", out)
	}
	if !containsEnv(out, "ZQK_PROJECT_ROOT=/wt") || containsEnv(out, "ZQK_PROJECT_ROOT=/old") {
		t.Fatalf("project root: %v", out)
	}
}

func containsEnv(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestLooksLikeSessionToken(t *testing.T) {
	if !LooksLikeSessionToken("ZS-123") || !LooksLikeSessionToken("ZQK-123") {
		t.Fatal("canonical and synonym session ids")
	}
	if LooksLikeSessionToken("ACC-1") || LooksLikeSessionToken("zqk-secret") {
		t.Fatal("account and opaque secret are not sessions")
	}
	if LooksLikeIssuedSecret("ZS-123") || LooksLikeIssuedSecret("ZQK-123") || LooksLikeIssuedSecret("ACC-1") {
		t.Fatal("session and ACC ids are not issued secrets")
	}
	if !LooksLikeIssuedSecret(SecretPrefix + "opaque") {
		t.Fatal("opaque issued secret")
	}
}
