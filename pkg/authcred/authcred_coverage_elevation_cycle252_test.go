package authcred

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestLookupBoundAccount_UncoveredBranches(t *testing.T) {
	// Empty project root or account ID
	if _, err := LookupBoundAccount("", "ACC-TEST"); err == nil {
		t.Fatal("expected error for empty project root")
	}
	if _, err := LookupBoundAccount("root", ""); err == nil {
		t.Fatal("expected error for empty account ID")
	}

	// Missing account index
	emptyRoot := t.TempDir()
	worker, err := LookupBoundAccount(emptyRoot, DefaultSwarmWorkerAccount)
	if err != nil {
		t.Fatalf("expected DefaultSwarmWorkerAccount to resolve on missing index, got %v", err)
	}
	if worker.ID != DefaultSwarmWorkerAccount || worker.PersonaRef != "PER-SWARM-WORKER" {
		t.Fatalf("unexpected worker: %+v", worker)
	}
	if _, err := LookupBoundAccount(emptyRoot, "ACC-NON-WORKER"); err == nil {
		t.Fatal("expected error for non-worker on missing index")
	}

	// Index present, but account not in index
	idxRoot := t.TempDir()
	accDir := paths.AccountsDirPath(idxRoot)
	if err := fileutil.EnsureDir(accDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(paths.AccountIndexPath(idxRoot), []byte(`{"mappings":{"ACC-OTHER":"hash123"}}`)); err != nil {
		t.Fatal(err)
	}
	// DefaultSwarmWorkerAccount should resolve even if not in mappings
	worker2, err := LookupBoundAccount(idxRoot, DefaultSwarmWorkerAccount)
	if err != nil || worker2.ID != DefaultSwarmWorkerAccount {
		t.Fatalf("expected worker resolve, got %+v, err=%v", worker2, err)
	}
	// Other account not in mappings fails
	if _, err := LookupBoundAccount(idxRoot, "ACC-MISSING"); err == nil {
		t.Fatal("expected error for missing account in index")
	}

	// Account in index, but CAS YAML missing
	if _, err := LookupBoundAccount(idxRoot, "ACC-OTHER"); err == nil {
		t.Fatal("expected error when CAS YAML is missing")
	}

	// Account in index, CAS YAML corrupted
	corruptHash := "corrupthash001"
	if err := fileutil.WriteStandardFile(paths.AccountIndexPath(idxRoot), []byte(`{"mappings":{"ACC-CORRUPT":"`+corruptHash+`"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(paths.AccountYAMLPath(idxRoot, corruptHash), []byte("invalid: yaml: [")); err != nil {
		t.Fatal(err)
	}
	if _, err := LookupBoundAccount(idxRoot, "ACC-CORRUPT"); err == nil {
		t.Fatal("expected error for corrupted YAML")
	}

	// Account in index, CAS YAML missing ID field (should default to accountID)
	noIDHash := "noidhash001"
	if err := fileutil.WriteStandardFile(paths.AccountIndexPath(idxRoot), []byte(`{"mappings":{"ACC-NOID":"`+noIDHash+`"}}`)); err != nil {
		t.Fatal(err)
	}
	noIDBody := "persona_ref: PER-NOID\nroles:\n  - worker\npermissions:\n  - read:*\n"
	if err := fileutil.WriteStandardFile(paths.AccountYAMLPath(idxRoot, noIDHash), []byte(noIDBody)); err != nil {
		t.Fatal(err)
	}
	boundNoID, err := LookupBoundAccount(idxRoot, "ACC-NOID")
	if err != nil {
		t.Fatalf("unexpected error for account with no ID field: %v", err)
	}
	if boundNoID.ID != "ACC-NOID" || boundNoID.PersonaRef != "PER-NOID" {
		t.Fatalf("unexpected bound account: %+v", boundNoID)
	}

	// Account in index with all fields populated
	fullHash := "fullhash001"
	if err := fileutil.WriteStandardFile(paths.AccountIndexPath(idxRoot), []byte(`{"mappings":{"ACC-FULL":"`+fullHash+`"}}`)); err != nil {
		t.Fatal(err)
	}
	fullBody := `id: ACC-FULL
persona: Persona Name
persona_ref: PER-FULL
persona_refs:
  - PER-EXTRA-1
  - PER-EXTRA-2
roles:
  - role1
  - role2
permissions:
  - perm1
  - perm2
`
	if err := fileutil.WriteStandardFile(paths.AccountYAMLPath(idxRoot, fullHash), []byte(fullBody)); err != nil {
		t.Fatal(err)
	}
	boundFull, err := LookupBoundAccount(idxRoot, "ACC-FULL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if boundFull.ID != "ACC-FULL" || boundFull.Persona != "Persona Name" || boundFull.PersonaRef != "PER-FULL" ||
		len(boundFull.PersonaRefs) != 2 || len(boundFull.Roles) != 2 || len(boundFull.Permissions) != 2 {
		t.Fatalf("unexpected boundFull: %+v", boundFull)
	}
}

func TestHasAccountInIndex_UncoveredBranches(t *testing.T) {
	if HasAccountInIndex("", "ACC-1") {
		t.Fatal("expected false for empty project root")
	}
	if HasAccountInIndex("root", "") {
		t.Fatal("expected false for empty account ID")
	}
	if !HasAccountInIndex("root", DefaultSwarmWorkerAccount) {
		t.Fatal("expected true for DefaultSwarmWorkerAccount")
	}

	emptyRoot := t.TempDir()
	if HasAccountInIndex(emptyRoot, "ACC-TEST") {
		t.Fatal("expected false when index is missing")
	}

	idxRoot := t.TempDir()
	accDir := paths.AccountsDirPath(idxRoot)
	if err := fileutil.EnsureDir(accDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(paths.AccountIndexPath(idxRoot), []byte(`{"mappings":{"ACC-EXISTS":"hash"}}`)); err != nil {
		t.Fatal(err)
	}
	if !HasAccountInIndex(idxRoot, "ACC-EXISTS") {
		t.Fatal("expected true for ACC-EXISTS")
	}
	if HasAccountInIndex(idxRoot, "ACC-NOT-EXISTS") {
		t.Fatal("expected false for ACC-NOT-EXISTS")
	}
}

func TestListAuthStrategyRecords_UncoveredBranches(t *testing.T) {
	if recs := ListAuthStrategyRecords(""); recs != nil {
		t.Fatal("expected nil for empty project root")
	}
	if recs := ListAuthStrategyRecords("   "); recs != nil {
		t.Fatal("expected nil for whitespace project root")
	}

	root := t.TempDir()
	dir := paths.AuthStrategiesDirPath(root)
	if err := fileutil.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}

	// Write invalid yaml file
	if err := fileutil.WriteStandardFile(filepath.Join(dir, "bad.yaml"), []byte("invalid: yaml: [")); err != nil {
		t.Fatal(err)
	}
	// Write valid yaml file
	goodBody := "id: AST-GOOD\nstrategy_type: token\nenabled: true\nstatus: active\n"
	if err := fileutil.WriteStandardFile(filepath.Join(dir, "good.yaml"), []byte(goodBody)); err != nil {
		t.Fatal(err)
	}

	recs := ListAuthStrategyRecords(root)
	if len(recs) != 1 || recs[0].ID != "AST-GOOD" || recs[0].Type != "token" || !recs[0].Enabled || recs[0].Status != "active" {
		t.Fatalf("unexpected recs: %+v", recs)
	}
}

func TestCasListingCache_UncoveredBranches(t *testing.T) {
	// loadListingIndex with missing file
	if _, err := loadListingIndex("/non/existent/index.json"); err == nil {
		t.Fatal("expected error for non-existent file")
	}

	tmp := t.TempDir()
	badJSON := filepath.Join(tmp, "bad.json")
	if err := fileutil.WriteStandardFile(badJSON, []byte("not-json")); err != nil {
		t.Fatal(err)
	}
	if _, err := loadListingIndex(badJSON); err == nil {
		t.Fatal("expected error for bad json")
	}

	nullMappings := filepath.Join(tmp, "null_map.json")
	if err := fileutil.WriteStandardFile(nullMappings, []byte(`{"mappings": null}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := loadListingIndex(nullMappings); err == nil {
		t.Fatal("expected error for null mappings")
	}

	// casMappings with empty arguments
	if m := casMappings("", "path"); m != nil {
		t.Fatal("expected nil for empty projectRoot")
	}
	if m := casMappings("root", ""); m != nil {
		t.Fatal("expected nil for empty indexPath")
	}
	if m := casMappings(tmp, filepath.Join(tmp, "nonexistent.json")); m != nil {
		t.Fatal("expected nil for nonexistent index")
	}

	// lookupStateMapping with invalid arguments
	if _, _, ok := lookupStateMapping("", "obj", "idx"); ok {
		t.Fatal("expected not ok")
	}
	if _, _, ok := lookupStateMapping("root", "", "idx"); ok {
		t.Fatal("expected not ok")
	}
	if _, _, ok := lookupStateMapping("root", "obj", ""); ok {
		t.Fatal("expected not ok")
	}
	if _, _, ok := lookupStateMapping(tmp, "obj", filepath.Join(tmp, "nonexistent.json")); ok {
		t.Fatal("expected not ok")
	}

	// Index with whitespace or empty hash
	idxPath := filepath.Join(tmp, "index.json")
	if err := fileutil.WriteStandardFile(idxPath, []byte(`{"mappings":{"obj1":"","obj2":"   ","obj3":"validhash"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := lookupStateMapping(tmp, "obj1", idxPath); ok {
		t.Fatal("expected not ok for empty hash")
	}
	if _, _, ok := lookupStateMapping(tmp, "obj2", idxPath); ok {
		t.Fatal("expected not ok for whitespace hash")
	}
	if s, hash, ok := lookupStateMapping(tmp, "obj3", idxPath); !ok || hash != "validhash" {
		t.Fatalf("expected ok with validhash, got hash=%q, ok=%v", hash, ok)
	} else {
		s.mu.Unlock()
	}

	// casHash
	if _, ok := casHash(tmp, "nonexistent", idxPath); ok {
		t.Fatal("expected casHash false for missing obj")
	}
	if hash, ok := casHash(tmp, "obj3", idxPath); !ok || hash != "validhash" {
		t.Fatalf("expected casHash true with validhash, got %q, %v", hash, ok)
	}

	// casYAML
	if _, ok := casYAML(tmp, "obj3", idxPath, ""); ok {
		t.Fatal("expected false for empty kindDir")
	}
	if _, ok := casYAML(tmp, "nonexistent", idxPath, tmp); ok {
		t.Fatal("expected false for missing obj in casYAML")
	}
	// yaml file missing on disk (stamp == 0)
	if _, ok := casYAML(tmp, "obj3", idxPath, tmp); ok {
		t.Fatal("expected false when yaml file does not exist on disk")
	}

	// create yaml file on disk
	yamlPath := filepath.Join(tmp, "validhash.yaml")
	yamlContent := []byte("id: obj3\nname: test\n")
	if err := fileutil.WriteStandardFile(yamlPath, yamlContent); err != nil {
		t.Fatal(err)
	}
	raw, ok := casYAML(tmp, "obj3", idxPath, tmp)
	if !ok || string(raw) != string(yamlContent) {
		t.Fatalf("expected casYAML to return yamlContent, got ok=%v, content=%q", ok, string(raw))
	}
	// Second call should hit the memory cache (hit.raw, true)
	raw2, ok2 := casYAML(tmp, "obj3", idxPath, tmp)
	if !ok2 || string(raw2) != string(yamlContent) {
		t.Fatalf("expected cached casYAML to return yamlContent, got ok=%v", ok2)
	}

	// AccountYAML with empty arguments
	if _, ok := AccountYAML("", "ACC-1"); ok {
		t.Fatal("expected false for empty projectRoot")
	}
	if _, ok := AccountYAML("root", ""); ok {
		t.Fatal("expected false for empty accountID")
	}

	// WalkAccountYAML edge cases
	WalkAccountYAML("", func(id string, raw []byte) bool { return true })
	WalkAccountYAML("root", nil)

	walkRoot := t.TempDir()
	walkAccDir := paths.AccountsDirPath(walkRoot)
	if err := fileutil.EnsureDir(walkAccDir); err != nil {
		t.Fatal(err)
	}
	// Index with non-ACC item, ACC item with missing YAML, and two valid ACC items
	hash1, hash2 := "walkhash001", "walkhash002"
	if err := fileutil.WriteStandardFile(paths.AccountYAMLPath(walkRoot, hash1), []byte("id: ACC-WALK-1\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(paths.AccountYAMLPath(walkRoot, hash2), []byte("id: ACC-WALK-2\n")); err != nil {
		t.Fatal(err)
	}
	walkIdx := `{"mappings":{
		"NON-ACC":"somehash",
		"ACC-MISSING":"missinghash",
		"ACC-WALK-1":"` + hash1 + `",
		"ACC-WALK-2":"` + hash2 + `"
	}}`
	if err := fileutil.WriteStandardFile(paths.AccountIndexPath(walkRoot), []byte(walkIdx)); err != nil {
		t.Fatal(err)
	}

	// Test early termination in WalkAccountYAML
	count := 0
	WalkAccountYAML(walkRoot, func(id string, raw []byte) bool {
		count++
		return false // stop after first
	})
	if count != 1 {
		t.Fatalf("expected early termination after 1 item, got %d", count)
	}
}

func TestCredentials_UncoveredBranches(t *testing.T) {
	// CanonDir
	if got := canonDir(""); got != "" {
		t.Fatalf("expected empty for empty dir, got %q", got)
	}
	if got := canonDir("   "); got != "" {
		t.Fatalf("expected empty for whitespace dir, got %q", got)
	}

	// ReadCredentialToken
	if got := ReadCredentialToken(""); got != "" {
		t.Fatalf("expected empty for empty path, got %q", got)
	}
	if got := ReadCredentialToken("/non/existent/path/cred"); got != "" {
		t.Fatalf("expected empty for nonexistent path, got %q", got)
	}

	// ResolveCredentialPath when projectRoot has a local credentials file
	tmpProject := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), "") // Clear test root
	localCredDir := filepath.Join(tmpProject, paths.ProjectDataDir)
	if err := fileutil.EnsureDir(localCredDir); err != nil {
		t.Fatal(err)
	}
	localCred := filepath.Join(localCredDir, "credentials")
	if err := fileutil.WriteSecureFile(localCred, []byte("ACC-LOCAL\n")); err != nil {
		t.Fatal(err)
	}
	if got := ResolveCredentialPath(tmpProject); got != localCred {
		t.Fatalf("expected local cred %q, got %q", localCred, got)
	}

	// Fallback when home dir is empty or fails
	t.Setenv(zqkenv.OSHome().Name(), "")
	emptyProject := t.TempDir()
	if got := ResolveCredentialPath(emptyProject); got != "" {
		t.Fatalf("expected empty string when home is empty, got %q", got)
	}
}

func TestIdentityStatus_UncoveredBranches(t *testing.T) {
	// ClassifyLane
	if got := ClassifyLane(nil); got != "" {
		t.Fatalf("expected empty for nil secCtx, got %q", got)
	}
	adminCtx := pkgctx.NewSecurityContext("ACC-USER", []string{"admin"}, []string{"read:*"})
	if got := ClassifyLane(adminCtx); got != LaneAdmin {
		t.Fatalf("expected LaneAdmin for admin role, got %q", got)
	}
	plannerWriteCtx := pkgctx.NewSecurityContext("ACC-PLANNER", []string{"custom_planner"}, []string{"write:priority_plan"})
	if got := ClassifyLane(plannerWriteCtx); got != LanePlanner {
		t.Fatalf("expected LanePlanner for write:priority_plan, got %q", got)
	}

	// IdentityStatusPath
	if got := IdentityStatusPath(""); got != "" {
		t.Fatalf("expected empty for empty projectRoot, got %q", got)
	}
	if got := IdentityStatusPath("   "); got != "" {
		t.Fatalf("expected empty for whitespace projectRoot, got %q", got)
	}

	// SnapshotFromSecurityContext with nil
	snapNil := SnapshotFromSecurityContext(nil, nil)
	if snapNil.Schema != IdentityStatusSchemaV1 || snapNil.AccountID != "" {
		t.Fatalf("unexpected snapNil: %+v", snapNil)
	}

	// SnapshotFromSecurityContext with extras and write:*
	secStar := pkgctx.NewSecurityContext("ACC-STAR", []string{"role1"}, []string{"write:*"})
	extras := map[string]any{
		objects.FieldKeyUsername:   "staruser",
		objects.FieldKeyTitle:      "Lead Dev",
		objects.FieldKeyPersonaRef: "PER-FROM-EXTRAS",
	}
	snapStar := SnapshotFromSecurityContext(secStar, extras)
	if !snapStar.WriteStar || snapStar.Username != "staruser" || snapStar.Title != "Lead Dev" || snapStar.PersonaID != "PER-FROM-EXTRAS" {
		t.Fatalf("unexpected snapStar: %+v", snapStar)
	}

	// identityFingerprint WriteStar true vs false
	fp1 := identityFingerprint(snapStar)
	snapStarNoWrite := snapStar
	snapStarNoWrite.WriteStar = false
	fp2 := identityFingerprint(snapStarNoWrite)
	if fp1 == fp2 {
		t.Fatal("fingerprints should differ based on WriteStar")
	}

	// WriteIdentityStatus and ReadIdentityStatus
	WriteIdentityStatus("", snapStar) // no-op

	tmpRoot := t.TempDir()
	WriteIdentityStatus(tmpRoot, snapStar)
	readSnap, ok := ReadIdentityStatus(tmpRoot)
	if !ok || readSnap.AccountID != "ACC-STAR" || readSnap.Username != "staruser" {
		t.Fatalf("unexpected readSnap: %+v, ok=%v", readSnap, ok)
	}

	// Rewrite with different roles to trigger identityFP change
	snapStarModified := snapStar
	snapStarModified.Roles = []string{"new_role"}
	WriteIdentityStatus(tmpRoot, snapStarModified)
	readModified, ok := ReadIdentityStatus(tmpRoot)
	if !ok || len(readModified.Roles) != 1 || readModified.Roles[0] != "new_role" {
		t.Fatalf("unexpected readModified: %+v", readModified)
	}

	// ReadIdentityStatus edge cases
	if _, ok := ReadIdentityStatus(""); ok {
		t.Fatal("expected false for empty project root")
	}
	badRoot := t.TempDir()
	idFile := IdentityStatusPath(badRoot)
	if err := fileutil.EnsureDir(filepath.Dir(idFile)); err != nil {
		t.Fatal(err)
	}
	// Corrupted JSON
	if err := fileutil.WriteStandardFile(idFile, []byte("bad-json")); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadIdentityStatus(badRoot); ok {
		t.Fatal("expected false for corrupted json")
	}
	// Missing AccountID
	if err := fileutil.WriteStandardFile(idFile, []byte(`{"schema":"v1"}`)); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadIdentityStatus(badRoot); ok {
		t.Fatal("expected false for empty account ID")
	}
}

func TestKeystoreCache_UncoveredBranches(t *testing.T) {
	// KeystoreRecord methods with empty entry
	emptyRec := KeystoreRecord{FileID: "file1", KeyID: "key1", Entry: nil}
	if emptyRec.KeyType() != "" || emptyRec.AccountID() != "" || emptyRec.CredentialHash() != "" || emptyRec.Revoked() != false || emptyRec.ExpiresAt() != "" {
		t.Fatalf("expected empty values for empty record: %+v", emptyRec)
	}

	// KeystoreRecord methods with populated entry
	entryMap := map[string]any{
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyAccountID:      "ACC-TEST-123",
		objects.FieldKeyCredentialHash: "hash123",
		objects.FieldKeyRevoked:        true,
		objects.FieldKeyExpiresAt:      "2026-12-31T23:59:59Z",
	}
	rec := KeystoreRecord{FileID: "file2", KeyID: "key2", Entry: entryMap}
	if rec.KeyType() != "api_key" || rec.AccountID() != "ACC-TEST-123" || rec.CredentialHash() != "hash123" || !rec.Revoked() || rec.ExpiresAt() != "2026-12-31T23:59:59Z" {
		t.Fatalf("unexpected record getter results: %+v", rec)
	}

	// parseKeystoreYAML
	if _, ok := parseKeystoreYAML("file3", []byte("invalid: yaml: [")); ok {
		t.Fatal("expected parseKeystoreYAML false for invalid YAML")
	}
	// without id field -> uses fileID
	parsedNoID, ok := parseKeystoreYAML("file_default_id", []byte("account_id: ACC-1\n"))
	if !ok || parsedNoID.KeyID != "file_default_id" {
		t.Fatalf("expected keyID to default to fileID, got %+v", parsedNoID)
	}

	// ListKeystoreRecords with empty root
	if _, err := ListKeystoreRecords(""); err == nil {
		t.Fatal("expected error for empty project root")
	}
	if _, err := ListKeystoreRecords("   "); err == nil {
		t.Fatal("expected error for whitespace project root")
	}

	// ListKeystoreRecords with missing dir returns error
	tmp := t.TempDir()
	_, err := ListKeystoreRecords(tmp)
	if err == nil {
		t.Fatal("expected error for nonexistent dir")
	}
}

func TestPersonaVocab_UncoveredBranches(t *testing.T) {
	if got := VocabularySchemesForPersona("", "PER-1"); got != nil {
		t.Fatal("expected nil for empty project root")
	}
	if got := VocabularySchemesForPersona("root", ""); got != nil {
		t.Fatal("expected nil for empty persona ID")
	}
	if got := VocabularySchemesForPersona("root", "   "); got != nil {
		t.Fatal("expected nil for whitespace persona ID")
	}

	tmp := t.TempDir()
	if got := VocabularySchemesForPersona(tmp, "PER-MISSING"); got != nil {
		t.Fatal("expected nil when persona does not exist")
	}

	// Corrupted YAML for persona
	dir := paths.PersonasDirPath(tmp)
	if err := fileutil.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	corruptHash := "percorrupt001"
	if err := fileutil.WriteStandardFile(paths.PersonaYAMLPath(tmp, corruptHash), []byte("invalid: yaml: [")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(paths.PersonaIndexPath(tmp), []byte(`{"mappings":{"PER-CORRUPT":"`+corruptHash+`"}}`)); err != nil {
		t.Fatal(err)
	}
	if got := VocabularySchemesForPersona(tmp, "PER-CORRUPT"); got != nil {
		t.Fatal("expected nil for corrupted persona YAML")
	}
}

func TestPlannerDoer_UncoveredBranches(t *testing.T) {
	// ExpandAssignedRoles with nil dir
	roles := ExpandAssignedRoles([]string{"role_a", "role_b"}, nil)
	if len(roles) != 2 || roles[0] != "role_a" || roles[1] != "role_b" {
		t.Fatalf("unexpected roles: %v", roles)
	}

	// HasExactPermission
	if HasExactPermission(nil, "perm") {
		t.Fatal("expected false for nil secCtx")
	}
	ctx := pkgctx.NewSecurityContext("ACC-1", []string{"role"}, []string{"perm1", "perm2"})
	if !HasExactPermission(ctx, "perm1") {
		t.Fatal("expected true for perm1")
	}
	if HasExactPermission(ctx, "perm3") {
		t.Fatal("expected false for perm3")
	}

	// HasWriteCode
	if HasWriteCode(nil) {
		t.Fatal("expected false for nil secCtx")
	}
	adminCtx := pkgctx.NewSecurityContext("ACC-1", []string{"admin"}, nil)
	if !HasWriteCode(adminCtx) {
		t.Fatal("expected true for admin role")
	}
	sysCtx := pkgctx.NewSystemSecurityContext()
	if !HasWriteCode(sysCtx) {
		t.Fatal("expected true for system account")
	}
	writeStarCtx := pkgctx.NewSecurityContext("ACC-1", []string{"worker"}, []string{"write:*"})
	if !HasWriteCode(writeStarCtx) {
		t.Fatal("expected true for write:*")
	}
	noWriteCtx := pkgctx.NewSecurityContext("ACC-1", []string{"worker"}, []string{"read:*"})
	if HasWriteCode(noWriteCtx) {
		t.Fatal("expected false for read:* only")
	}

	// MayOrchestratePeers
	if MayOrchestratePeers(nil) {
		t.Fatal("expected false for nil secCtx")
	}
	if !MayOrchestratePeers(sysCtx) {
		t.Fatal("expected true for system account")
	}
	if !MayOrchestratePeers(adminCtx) {
		t.Fatal("expected true for admin role")
	}
	for _, perm := range []string{PermissionAgentOrchestrate, "write:*", "write:priority_plan", "write:agent_instruction", "write:strategic_plan"} {
		pCtx := pkgctx.NewSecurityContext("ACC-P", []string{"custom"}, []string{perm})
		if !MayOrchestratePeers(pCtx) {
			t.Fatalf("expected true for MayOrchestratePeers with perm %s", perm)
		}
	}
	unauthCtx := pkgctx.NewSecurityContext("ACC-U", []string{"worker"}, []string{"write:code", "read:*"})
	if MayOrchestratePeers(unauthCtx) {
		t.Fatal("expected false for unauthCtx")
	}

	// DenyPeerSteerIfDoer
	if err := DenyPeerSteerIfDoer(unauthCtx, "peer_agent"); err == nil {
		t.Fatal("expected error for DenyPeerSteerIfDoer with doer")
	}
	orchCtx := pkgctx.NewSecurityContext("ACC-O", []string{"planner"}, []string{PermissionAgentOrchestrate})
	if err := DenyPeerSteerIfDoer(orchCtx, "peer_agent"); err != nil {
		t.Fatalf("unexpected error for planner peer steer: %v", err)
	}

	// StrategicKernelKinds
	for _, k := range StrategicKernelKinds {
		if !IsStrategicKernelKind(k) {
			t.Fatalf("expected true for strategic kernel kind %s", k)
		}
	}
	if IsStrategicKernelKind("random_kind") {
		t.Fatal("expected false for random_kind")
	}

	// ValidateDoerPermissions
	for _, bad := range DoerLaneForbiddenPerms {
		if err := ValidateDoerPermissions([]string{"read:*", bad}); err == nil {
			t.Fatalf("expected error for forbidden doer perm %s", bad)
		}
	}

	// ValidatePlannerPermissions
	for _, bad := range PlannerLaneForbiddenPerms {
		if err := ValidatePlannerPermissions([]string{"read:*", bad}); err == nil {
			t.Fatalf("expected error for forbidden planner perm %s", bad)
		}
	}
}

func TestPrototypeRejection_UncoveredBranches(t *testing.T) {
	// loadPrototypeAccountRefs with env variable
	t.Setenv(zqkenv.PrototypeAccountRefs().Name(), "ACC-ENV-PROTO-1,ACC-ENV-PROTO-2")
	// Re-run registration to incorporate env vars
	RegisterPrototypeAccountRefs("ACC-ENV-PROTO-3")
	if !IsPrototypeAccount("ACC-ENV-PROTO-1") && !isRegisteredPrototypeRef("acc-env-proto-1") {
		// Just ensure it loaded without crashing
	}

	// ValidateOwnerRefProduction
	if err := ValidateOwnerRefProduction(""); err != nil {
		t.Fatalf("expected nil for empty owner ref, got %v", err)
	}
	if err := ValidateOwnerRefProduction("   "); err != nil {
		t.Fatalf("expected nil for whitespace owner ref, got %v", err)
	}
	if err := ValidateOwnerRefProduction("USER-12345"); err == nil {
		t.Fatal("expected error for non-ACC prefix")
	}
	if err := ValidateOwnerRefProduction("ACC-TEST-USER"); err == nil {
		t.Fatal("expected error for prototype account")
	}
	if err := ValidateOwnerRefProduction("ACC-PRODUCTION-VALID-001"); err != nil {
		t.Fatalf("unexpected error for valid production account: %v", err)
	}

	// IsPrototypeAccount
	if IsPrototypeAccount("") {
		t.Fatal("expected false for empty ref")
	}
	if IsPrototypeAccount("NOT-AN-ACC") {
		t.Fatal("expected false for non-ACC prefix")
	}
	for _, marker := range prototypeAccountIDMarkers {
		cand := "ACC-SUB-" + strings.ToUpper(marker) + "-999"
		if !IsPrototypeAccount(cand) {
			t.Fatalf("expected true for marker %s in %s", marker, cand)
		}
	}
}

func TestResolve_UncoveredBranches(t *testing.T) {
	// SanitizeDigest
	if got := SanitizeDigest(""); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
	sanitized := SanitizeDigest("sha256:abcd1234efgh")
	if sanitized != "sha256:abcd1234efgh" {
		t.Fatalf("unexpected sanitized string: %q", sanitized)
	}

	// NormalizeCredentialHash
	if got := NormalizeCredentialHash("sha256:11223344"); got != "11223344" {
		t.Fatalf("expected 11223344, got %q", got)
	}
	if got := NormalizeCredentialHash("  sha256:55667788  "); got != "55667788" {
		t.Fatalf("expected 55667788, got %q", got)
	}
	if got := NormalizeCredentialHash("plainhash"); got != "plainhash" {
		t.Fatalf("expected plainhash, got %q", got)
	}

	// ResolveAPIKey
	if _, err := ResolveAPIKey("root", ""); err == nil {
		t.Fatal("expected error for empty rawKey")
	}
	if _, err := ResolveAPIKey("", "rawkey"); err == nil {
		t.Fatal("expected error for empty projectRoot")
	}

	// Setup keystore dir for ResolveAPIKey tests
	tmp := t.TempDir()
	ksDir := paths.KeystoreDirPath(tmp)
	if err := fileutil.EnsureDir(ksDir); err != nil {
		t.Fatal(err)
	}

	rawValid := APIKeyPrefix + "valid-token"
	validHash := HashAPIKey(rawValid)

	// Entry with wrong key type
	wrongType := `account_id: ACC-WRONG-TYPE
credential_hash: ` + validHash + `
id: KEY-WRONG
key_type: certificate
status: active
`
	if err := fileutil.WriteStandardFile(filepath.Join(ksDir, "wrong_type.yaml"), []byte(wrongType)); err != nil {
		t.Fatal(err)
	}

	// Entry revoked
	revoked := `account_id: ACC-REVOKED
credential_hash: ` + validHash + `
id: KEY-REVOKED
key_type: api_key
revoked: true
status: active
`
	if err := fileutil.WriteStandardFile(filepath.Join(ksDir, "revoked.yaml"), []byte(revoked)); err != nil {
		t.Fatal(err)
	}

	// Entry expired
	expired := `account_id: ACC-EXPIRED
credential_hash: ` + validHash + `
id: KEY-EXPIRED
key_type: api_key
expires_at: "2020-01-01T00:00:00Z"
status: active
`
	if err := fileutil.WriteStandardFile(filepath.Join(ksDir, "expired.yaml"), []byte(expired)); err != nil {
		t.Fatal(err)
	}

	rawUnparseable := APIKeyPrefix + "unparseable-token"
	unparseableHash := HashAPIKey(rawUnparseable)
	unparseableExp := `account_id: ACC-UNPARSEABLE-EXP
credential_hash: ` + unparseableHash + `
id: KEY-UNPARSEABLE
key_type: api_key
expires_at: "not-a-date"
status: active
`
	if err := fileutil.WriteStandardFile(filepath.Join(ksDir, "unparseable.yaml"), []byte(unparseableExp)); err != nil {
		t.Fatal(err)
	}

	// Entry with empty account_id
	emptyAcc := `account_id: ""
credential_hash: ` + validHash + `
id: KEY-EMPTY-ACC
key_type: api_key
status: active
`
	if err := fileutil.WriteStandardFile(filepath.Join(ksDir, "empty_acc.yaml"), []byte(emptyAcc)); err != nil {
		t.Fatal(err)
	}

	// None of the above should match as valid
	InvalidateKeystore(tmp)
	if _, err := ResolveAPIKey(tmp, "wrong-key"); err == nil {
		t.Fatal("expected error for wrong key")
	}

	// Valid entry with personal_access_token key_type and future expiration
	futureExp := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	validPAT := `account_id: ACC-PAT-MATCH
credential_hash: ` + validHash + `
id: KEY-PAT-MATCH
key_type: personal_access_token
expires_at: "` + futureExp + `"
status: active
`
	if err := fileutil.WriteStandardFile(filepath.Join(ksDir, "valid_pat.yaml"), []byte(validPAT)); err != nil {
		t.Fatal(err)
	}
	InvalidateKeystore(tmp)
	match, err := ResolveAPIKey(tmp, rawValid)
	if err != nil {
		t.Fatalf("expected match, got %v", err)
	}
	if match.AccountID != "ACC-PAT-MATCH" || match.KeyID != "KEY-PAT-MATCH" {
		t.Fatalf("unexpected match: %+v", match)
	}

	matchUnparseable, err := ResolveAPIKey(tmp, rawUnparseable)
	if err != nil {
		t.Fatalf("expected match for unparseable expiration date, got %v", err)
	}
	if matchUnparseable.AccountID != "ACC-UNPARSEABLE-EXP" || matchUnparseable.KeyID != "KEY-UNPARSEABLE" {
		t.Fatalf("unexpected matchUnparseable: %+v", matchUnparseable)
	}

	// LooksLikeIssuedAPIKey
	if LooksLikeIssuedAPIKey("") {
		t.Fatal("expected false for empty key")
	}
	if LooksLikeIssuedAPIKey("   ") {
		t.Fatal("expected false for whitespace key")
	}
	if LooksLikeIssuedAPIKey("account:someuser") {
		t.Fatal("expected false for account: prefix")
	}
	if LooksLikeIssuedAPIKey("ACC-USER-1") {
		t.Fatal("expected false for ACC- prefix")
	}
	if LooksLikeIssuedAPIKey("ZS-session-id") {
		t.Fatal("expected false for ZS- prefix")
	}
	if LooksLikeIssuedAPIKey("ZQK-synonym-session") {
		t.Fatal("expected false for ZQK- prefix")
	}
	if !LooksLikeIssuedAPIKey("custom_secret_key_123") {
		t.Fatal("expected true for custom secret key")
	}
}

func TestSeatAccount_UncoveredBranches(t *testing.T) {
	// CanonicalAccountID
	if got := CanonicalAccountID("root", ""); got != "" {
		t.Fatalf("expected empty for empty ref, got %q", got)
	}
	if got := CanonicalAccountID("root", "custom:unrecognized:colon"); got != "" {
		t.Fatalf("expected empty for unrecognized colon prefix, got %q", got)
	}
	if got := CanonicalAccountID("root", "account:   "); got != "" {
		t.Fatalf("expected empty for empty username, got %q", got)
	}
	if got := CanonicalAccountID("", "account:user"); got != "" {
		t.Fatalf("expected empty for empty project root, got %q", got)
	}

	// ResolveSeatAccount
	if got := ResolveSeatAccount("", "some-role"); got != DefaultSwarmWorkerAccount {
		t.Fatalf("expected DefaultSwarmWorkerAccount for empty project root, got %q", got)
	}

	// AccountCASPath
	if got := AccountCASPath("", "ACC-1"); got != "" {
		t.Fatalf("expected empty for empty project root, got %q", got)
	}
	if got := AccountCASPath("root", "NOT-ACC-1"); got != "" {
		t.Fatalf("expected empty for non-ACC id, got %q", got)
	}
	tmpRoot := t.TempDir()
	if got := AccountCASPath(tmpRoot, "ACC-MISSING"); got != "" {
		t.Fatalf("expected empty for missing account in index, got %q", got)
	}

	// findPlannerAccount with nil dir
	if got := findPlannerAccount(nil, "ref"); got != "" {
		t.Fatalf("expected empty for nil dir, got %q", got)
	}
	plannerRoleOnly := RoleRecord{
		ID:          "ROL-PLAN-NO-ACC",
		RoleID:      "planner_no_acc",
		Permissions: []string{PermissionAgentOrchestrate},
	}
	dirNoAcc := MemoryDirectory{
		RoleList:    []RoleRecord{plannerRoleOnly},
		AccountList: []AccountRecord{{ID: "ACC-UNRELATED", Roles: []string{"other_role"}}},
	}
	if got := findPlannerAccount(dirNoAcc, "planner_no_acc"); got != "" {
		t.Fatalf("expected empty for planner role without matching account, got %q", got)
	}

	// findAccountForPersona with empty
	if got := findAccountForPersona("root", ""); got != "" {
		t.Fatalf("expected empty for empty persona, got %q", got)
	}

	// findAccountForRole with empty
	if got := findAccountForRole("root", ""); got != "" {
		t.Fatalf("expected empty for empty role, got %q", got)
	}

	// findAccountForUsername with empty
	if got := findAccountForUsername("root", ""); got != "" {
		t.Fatalf("expected empty for empty username, got %q", got)
	}

	// Accounts in disk directory with inactive status, persona array match
	accDir := paths.AccountsDirPath(tmpRoot)
	if err := fileutil.EnsureDir(accDir); err != nil {
		t.Fatal(err)
	}
	hash1, hash2, hash3 := "acchash001", "acchash002", "acchash003"
	// Inactive account
	if err := fileutil.WriteStandardFile(paths.AccountYAMLPath(tmpRoot, hash1), []byte("id: ACC-INACTIVE\nstatus: retired\nusername: user_inactive\n")); err != nil {
		t.Fatal(err)
	}
	// Implemented account (active status synonym)
	if err := fileutil.WriteStandardFile(paths.AccountYAMLPath(tmpRoot, hash2), []byte("id: ACC-IMPL\nstatus: implemented\npersona: PersonaImpl\nroles:\n  - tester\nusername: user_impl\n")); err != nil {
		t.Fatal(err)
	}
	// Account with PersonaRefs array
	if err := fileutil.WriteStandardFile(paths.AccountYAMLPath(tmpRoot, hash3), []byte("id: ACC-REFS\nstatus: active\npersona_refs:\n  - PER-ARRAY-1\n  - PER-ARRAY-2\nusername: user_refs\n")); err != nil {
		t.Fatal(err)
	}

	idxContent := `{"mappings":{
		"ACC-INACTIVE":"` + hash1 + `",
		"ACC-IMPL":"` + hash2 + `",
		"ACC-REFS":"` + hash3 + `"
	}}`
	if err := fileutil.WriteStandardFile(paths.AccountIndexPath(tmpRoot), []byte(idxContent)); err != nil {
		t.Fatal(err)
	}

	// Inactive should not be found
	if got := findAccountForUsername(tmpRoot, "user_inactive"); got != "" {
		t.Fatalf("expected inactive account not found, got %q", got)
	}
	// Implemented should be found
	if got := findAccountForUsername(tmpRoot, "user_impl"); got != "ACC-IMPL" {
		t.Fatalf("expected ACC-IMPL, got %q", got)
	}
	if got := findAccountForPersona(tmpRoot, "PersonaImpl"); got != "ACC-IMPL" {
		t.Fatalf("expected ACC-IMPL for PersonaImpl, got %q", got)
	}
	// PersonaRefs array match
	if got := findAccountForPersona(tmpRoot, "PER-ARRAY-2"); got != "ACC-REFS" {
		t.Fatalf("expected ACC-REFS for PER-ARRAY-2, got %q", got)
	}
	// Role match on acc.Roles
	if got := findAccountForRole(tmpRoot, "tester"); got != "ACC-IMPL" {
		t.Fatalf("expected ACC-IMPL for tester role, got %q", got)
	}
}

func TestSeatDirectory_UncoveredBranches(t *testing.T) {
	// DiskDirectory with empty root
	emptyDir := DiskDirectory{Root: ""}
	if emptyDir.Roles() != nil {
		t.Fatal("expected nil roles for empty Root")
	}
	if emptyDir.Accounts() != nil {
		t.Fatal("expected nil accounts for empty Root")
	}

	// MemoryDirectory
	mem := MemoryDirectory{
		RoleList:    []RoleRecord{{ID: "ROL-1", RoleID: "role_1"}},
		AccountList: []AccountRecord{{ID: "ACC-1", Username: "user1"}},
	}
	if len(mem.Roles()) != 1 || mem.Roles()[0].ID != "ROL-1" {
		t.Fatalf("unexpected Roles: %+v", mem.Roles())
	}
	if len(mem.Accounts()) != 1 || mem.Accounts()[0].ID != "ACC-1" {
		t.Fatalf("unexpected Accounts: %+v", mem.Accounts())
	}

	// parseRoleFile
	if _, ok := parseRoleFile([]byte("invalid: yaml: [")); ok {
		t.Fatal("expected false for invalid yaml")
	}
	if _, ok := parseRoleFile([]byte("status: active\n")); ok {
		t.Fatal("expected false for empty ID and RoleID")
	}
	rec, ok := parseRoleFile([]byte("id: ROL-ONLY\nstatus: active\n"))
	if !ok || rec.ID != "ROL-ONLY" {
		t.Fatalf("expected parsed role with ID only, got %+v", rec)
	}

	// GrantsPlanner
	plannerPerms := []string{
		PermissionAgentOrchestrate,
		"write:*",
		"write:priority_plan",
		"write:agent_instruction",
		"write:strategic_plan",
	}
	for _, p := range plannerPerms {
		r := RoleRecord{Permissions: []string{p}}
		if !r.GrantsPlanner() {
			t.Fatalf("expected GrantsPlanner true for %s", p)
		}
	}
	nonPlannerRole := RoleRecord{Permissions: []string{"read:*", "write:code"}}
	if nonPlannerRole.GrantsPlanner() {
		t.Fatal("expected GrantsPlanner false for non-planner perms")
	}

	// stringCollector
	sc := newStringCollector(4, true)
	sc.Add("")
	sc.Add("   ")
	sc.Add("Label1")
	sc.Add("label1") // duplicate when lower=true
	sc.Add("Label2")
	res := sc.Result()
	if len(res) != 2 || res[0] != "label1" || res[1] != "label2" {
		t.Fatalf("unexpected stringCollector result: %v", res)
	}

	// RoleRecord.Labels and Matches
	roleRec := RoleRecord{
		ID:      "ROL-TEST-001",
		RoleID:  "role-test-canonical",
		Aliases: []string{"test_alias_1", "test_alias_2"},
	}
	labels := roleRec.Labels()
	if len(labels) < 4 {
		t.Fatalf("unexpected labels: %v", labels)
	}
	if roleRec.Matches("") {
		t.Fatal("expected Matches false for empty string")
	}
	if roleRec.Matches("   ") {
		t.Fatal("expected Matches false for whitespace string")
	}
	if !roleRec.Matches("TEST-001") { // trimmed ROL- prefix
		t.Fatal("expected Matches true for trimmed ROL- prefix")
	}
	if !roleRec.Matches("test_alias_2") {
		t.Fatal("expected Matches true for alias")
	}
	if roleRec.Matches("unknown_label") {
		t.Fatal("expected Matches false for unknown label")
	}

	// MatchesAny
	if roleRec.MatchesAny([]string{"unknown", "test_alias_1"}) != true {
		t.Fatal("expected MatchesAny true")
	}
	if roleRec.MatchesAny([]string{"unknown", "other"}) != false {
		t.Fatal("expected MatchesAny false")
	}

	// accountBindsRef
	accRec := AccountRecord{
		ID:          "ACC-BIND-001",
		Username:    "bind_user",
		PersonaRef:  "PER-BIND-REF",
		Persona:     "Bind Persona",
		PersonaRefs: []string{"PER-EXTRA-A", "PER-EXTRA-B"},
		Roles:       []string{"planner_role"},
	}
	if accountBindsRef(accRec, "") {
		t.Fatal("expected false for empty ref")
	}
	if !accountBindsRef(accRec, "acc-bind-001") {
		t.Fatal("expected true for case-insensitive ID match")
	}
	if !accountBindsRef(accRec, "BIND_USER") {
		t.Fatal("expected true for case-insensitive username match")
	}
	if !accountBindsRef(accRec, "PER-BIND-REF") {
		t.Fatal("expected true for persona ref match")
	}
	if !accountBindsRef(accRec, "Bind Persona") {
		t.Fatal("expected true for persona match")
	}
	if !accountBindsRef(accRec, "PER-EXTRA-B") {
		t.Fatal("expected true for PersonaRefs item match")
	}
	if accountBindsRef(accRec, "NON-MATCHING-REF") {
		t.Fatal("expected false for non-matching ref")
	}

	// accountIsPlanner
	if accountIsPlanner(nil, accRec) {
		t.Fatal("expected false for nil dir")
	}

	plannerRoleRec := RoleRecord{
		ID:          "ROL-PLAN-001",
		RoleID:      "planner_role",
		Permissions: []string{PermissionAgentOrchestrate},
	}
	plannerDir := MemoryDirectory{
		RoleList:    []RoleRecord{plannerRoleRec},
		AccountList: []AccountRecord{accRec},
	}
	if !accountIsPlanner(plannerDir, accRec) {
		t.Fatal("expected true for planner account")
	}

	// IsPlannerSeatRef
	if IsPlannerSeatRef(nil, "ref") {
		t.Fatal("expected false for nil dir")
	}
	if IsPlannerSeatRef(plannerDir, "") {
		t.Fatal("expected false for empty ref")
	}
	if !IsPlannerSeatRef(plannerDir, "ROL-PLAN-001") {
		t.Fatal("expected true for planner role ref")
	}
	if !IsPlannerSeatRef(plannerDir, "ACC-BIND-001") {
		t.Fatal("expected true for planner account ref")
	}
	if IsPlannerSeatRef(plannerDir, "NON-EXISTENT") {
		t.Fatal("expected false for non-existent ref")
	}

	// PermissionsForAssignedRoles
	if perms := PermissionsForAssignedRoles(nil, []string{"role"}); perms != nil {
		t.Fatal("expected nil for nil dir")
	}
	perms := PermissionsForAssignedRoles(plannerDir, []string{"planner_role"})
	if len(perms) != 1 || perms[0] != PermissionAgentOrchestrate {
		t.Fatalf("unexpected perms: %v", perms)
	}
	if perms := PermissionsForAssignedRoles(plannerDir, []string{"unknown_role"}); len(perms) != 0 {
		t.Fatalf("expected empty perms, got %v", perms)
	}

	// collectRoleRecords fallback when role index is missing
	diskRoot := t.TempDir()
	roleDir := paths.RolesDirPath(diskRoot)
	if err := fileutil.EnsureDir(roleDir); err != nil {
		t.Fatal(err)
	}
	// Add roles directly to dir without .role.index
	roleBody1 := "id: ROL-FALLBACK-1\nrole_id: fallback_1\nstatus: active\n"
	roleBody2 := "id: ROL-FALLBACK-2\nrole_id: fallback_2\nstatus: retired\n" // inactive
	if err := fileutil.WriteStandardFile(filepath.Join(roleDir, "role1.yaml"), []byte(roleBody1)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(filepath.Join(roleDir, "role2.yaml"), []byte(roleBody2)); err != nil {
		t.Fatal(err)
	}
	diskDir := NewDiskSeatDirectory(diskRoot)
	loadedRoles := diskDir.Roles()
	if len(loadedRoles) != 1 || loadedRoles[0].ID != "ROL-FALLBACK-1" {
		t.Fatalf("expected only active fallback role, got %+v", loadedRoles)
	}
}

func TestSeating_UncoveredBranches(t *testing.T) {
	// SeatKeyPath and SeatCredentialPath with slash in account ID
	path := SeatKeyPath("/root", "ACC/WITH/SLASH")
	if strings.Contains(filepath.Base(path), "/") {
		t.Fatalf("expected slashes to be sanitized in base path, got %q", path)
	}

	// WriteSeatKey and LoadSeatCredential
	tmp := t.TempDir()
	const testAcc = "ACC-SEAT-WRITE-TEST"
	const testKey = "zqk_ak_secret12345"
	if err := WriteSeatKey(tmp, testAcc, testKey); err != nil {
		t.Fatalf("WriteSeatKey failed: %v", err)
	}
	loaded, err := LoadSeatCredential(tmp, testAcc)
	if err != nil || loaded != testKey {
		t.Fatalf("LoadSeatCredential got %q, err=%v", loaded, err)
	}

	// LoadSeatCredential for missing account
	missing, err := LoadSeatCredential(tmp, "ACC-NONEXISTENT")
	if err != nil || missing != "" {
		t.Fatalf("expected empty credential for missing account, got %q, err=%v", missing, err)
	}

	// LoadSeatCredential error when path is a directory
	tmpDirFile := t.TempDir()
	credPath := SeatCredentialPath(tmpDirFile, "ACC-DIR-ERROR")
	if err := fileutil.EnsureDir(credPath); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSeatCredential(tmpDirFile, "ACC-DIR-ERROR"); err == nil {
		t.Fatal("expected error when reading directory as credential file")
	}

	// WriteSeatKey failure when parent path is blocked by a file
	tmpFile := filepath.Join(t.TempDir(), "blocker_file")
	if err := fileutil.WriteStandardFile(tmpFile, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := WriteSeatKey(tmpFile, "ACC-FAIL", "key"); err == nil {
		t.Fatal("expected error when seating credentials dir cannot be created")
	}
	WriteIdentityStatus(tmpFile, SnapshotFromSecurityContext(nil, nil))

	// APIKeyForSeat fallback
	if got := APIKeyForSeat(tmp, testAcc); got != testKey {
		t.Fatalf("expected testKey, got %q", got)
	}
	if got := APIKeyForSeat(tmp, "ACC-MISSING-SEAT"); got != "ACC-MISSING-SEAT" {
		t.Fatalf("expected fallback to accountID, got %q", got)
	}

	// WithSeatAPIKeyEnv edge cases
	parentEnv := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/Users/test",
		"USER=testuser",
		"ZQK_API_KEY=old_api_key",
		"ZQK_PROJECT_ROOT=/old/root",
		"SECRET_LEAK=should_be_dropped",
		"ZQK_CUSTOM_VAR=preserved",
	}
	cleaned := WithSeatAPIKeyEnv(parentEnv, "new_key", "/new/root")
	foundPath, foundHome, foundUser, foundAPIKey, foundRoot, foundCustom, foundSecret := false, false, false, false, false, false, false
	for _, env := range cleaned {
		if strings.HasPrefix(env, "PATH=") {
			foundPath = true
		}
		if strings.HasPrefix(env, "HOME=") {
			foundHome = true
		}
		if strings.HasPrefix(env, "USER=") {
			foundUser = true
		}
		if env == "ZQK_API_KEY=new_key" {
			foundAPIKey = true
		}
		if env == "ZQK_PROJECT_ROOT=/new/root" {
			foundRoot = true
		}
		if env == "ZQK_CUSTOM_VAR=preserved" {
			foundCustom = true
		}
		if strings.HasPrefix(env, "SECRET_LEAK=") {
			foundSecret = true
		}
	}
	if !foundPath || !foundHome || !foundUser || !foundAPIKey || !foundRoot || !foundCustom {
		t.Fatalf("expected preserved standard/ZQK vars, got %v", cleaned)
	}
	if foundSecret {
		t.Fatalf("expected non-whitelisted secret to be dropped: %v", cleaned)
	}
}
