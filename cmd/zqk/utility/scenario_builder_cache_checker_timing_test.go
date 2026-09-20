package utility

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// newTimingTestScenarioBuilder builds an isolated project root, storage, coordinator, and a
// [ScenarioBuilder] with infrastructure set up, returning the builder and its storage — the shared
// preamble for cache-checker timing tests, which differ only in when they clear the cache checker.
//
// Uses ForTest storage: full NewStorageFactory wires global audit/async paths and can block wg.Wait()
// under bundle -parallel load until the package times out (see SCH-run-cmd-zqk-utility-0).
func newTimingTestScenarioBuilder(t *testing.T) (*ScenarioBuilder, *storagepkg.FileObjectStorage) {
	t.Helper()
	tmpDir := t.TempDir()
	// Bind isolated TEST_ROOT before fallthrough is allowed — otherwise ApplyIsolatedStorageEnv
	// would either dial live PW or (with unconditional fallthrough) write into live .zqk/process.
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Setenv(zqkenv.ProjectRoot().Name(), "")
	zqkenv.ApplyIsolatedStorageEnv(t.Setenv)
	if err := fileutil.MkdirAll(datacell.ProcessPrimaryDir(tmpDir), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process directory: %v", err)
	}
	storageProvider, err := storagepkg.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	testkit.RegisterTempProjectTeardown(t, tmpDir, storageProvider)

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	builder := NewScenarioBuilder(tmpDir, storageProvider, coordinator)
	builder.logger = logging.NewEventLogger(pkgctx.NewSystemContext())
	builder.secCtx = pkgctx.NewSystemSecurityContext()
	if builder.config == nil {
		builder.config = &ScenarioBuilderConfig{}
	}
	builder.config.TargetDir = tmpDir
	if err := builder.setupInfrastructure(); err != nil {
		t.Fatalf("Failed to setup infrastructure: %v", err)
	}
	return builder, storageProvider
}

// TestCacheCheckerTiming_RaceCondition exercises clearing SetCacheChecker before workstream Create
// after validate (premature-clear scenario). Uses sequential creates: parallel account+workstream
// Create previously wedged wg.Wait until bundle timeout (SCH-run-cmd-zqk-utility-0).
// Do not use t.Parallel(): storage.SetCacheChecker is process-global.
func TestCacheCheckerTiming_RaceCondition(t *testing.T) {
	builder, storageProvider := newTimingTestScenarioBuilder(t)

	// Create test objects with dependencies (account -> workstream)
	testObjects := []map[string]any{
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyID:       "ACC-1785920548450214015-3df55bd1",
			objects.FieldKeyTitle:    "Test User",
			objects.FieldKeyUsername: "test-user",
			objects.FieldKeyStatus:   scenarioBuilderStatusActive,
		},
		{
			objects.FieldKeyKind:       "workstream",
			objects.FieldKeyID:         "WS-TEST-001",
			objects.FieldKeyTitle:      "Test Workstream",
			objects.FieldKeyStatus:     scenarioBuilderStatusActive,
			objects.FieldKeyEntryPoint: "main",                             // Required field
			objects.FieldKeyOwnerRef:   "ACC-1785920548450214015-3df55bd1", // References account created in same batch
		},
	}

	// Pre-populate reference cache (like scenario builder does)
	referenceCache := make(map[string]bool)
	referenceCacheMu := &sync.Mutex{}
	idStream := make(map[string]string)
	idStreamMu := &sync.Mutex{}

	for _, obj := range testObjects {
		if objID, ok := obj[objects.FieldKeyID].(string); ok && objID != emptyValue {
			referenceCacheMu.Lock()
			referenceCache[objID] = true
			referenceCacheMu.Unlock()
		}
		// Also cache account username format
		if kind, ok := obj[objects.FieldKeyKind].(string); ok && kind == "account" {
			if username, ok := obj[objects.FieldKeyUsername].(string); ok && username != emptyValue {
				accountID := "" // ACC-* from storage;
				referenceCacheMu.Lock()
				referenceCache[accountID] = true
				referenceCacheMu.Unlock()
			}
		}
	}

	// Set cache checker (like scenario builder does)
	storagepkg.SetCacheChecker(func(objectID string) (string, bool) {
		// Check ID stream
		idStreamMu.Lock()
		if _, exists := idStream[objectID]; exists {
			idStreamMu.Unlock()
			return "", true // Exists in stream
		}
		idStreamMu.Unlock()

		// Check reference cache
		referenceCacheMu.Lock()
		exists, cached := referenceCache[objectID]
		referenceCacheMu.Unlock()
		if cached && exists {
			return "", true // Exists in cache
		}

		return "", false
	})

	// Sequential flow: parallel account+workstream Create + wg.Wait wedged until package timeout
	// under scheduler bundles (SCH-run-cmd-zqk-utility-0). Reproduce "clear checker before
	// persistence" without goroutines: validate workstream while checker is set, clear, then Create.
	ctx := pkgctx.NewSystemContext()
	accObj := testObjects[0]
	wsObj := testObjects[1]

	if err := builder.prepareObjectFromDataFile(accObj, "account"); err != nil {
		t.Fatalf("prepare account: %v", err)
	}
	if err := builder.validateObjectBeforeCreation(accObj, "account"); err != nil {
		t.Fatalf("validate account: %v", err)
	}
	accID, _ := accObj[objects.FieldKeyID].(string)
	accCtx := storagepkg.WithSyncCreateForKind(pkgctx.WithCacheUpdate(ctx, accID, "account", ""), "account")
	if err := storageProvider.Create(accCtx, builder.secCtx, accObj); err != nil {
		t.Fatalf("create account: %v", err)
	}
	idStreamMu.Lock()
	idStream[accID] = accID
	idStreamMu.Unlock()

	if err := builder.prepareObjectFromDataFile(wsObj, "workstream"); err != nil {
		t.Fatalf("prepare workstream: %v", err)
	}
	if err := builder.validateObjectBeforeCreation(wsObj, "workstream"); err != nil {
		t.Fatalf("validate workstream: %v", err)
	}

	storagepkg.SetCacheChecker(nil)

	wsID, _ := wsObj[objects.FieldKeyID].(string)
	wsCtx := storagepkg.WithSyncCreateForKind(pkgctx.WithCacheUpdate(ctx, wsID, "workstream", ""), "workstream")
	errWS := storageProvider.Create(wsCtx, builder.secCtx, wsObj)
	if errWS != nil {
		t.Logf("workstream create after deliberate checker clear (expected failure path): %v", errWS)
	} else {
		idStreamMu.Lock()
		idStream[wsID] = wsID
		idStreamMu.Unlock()
	}

	storagepkg.SetCacheChecker(nil)

	if errWS == nil {
		for _, obj := range testObjects {
			objID, _ := obj[objects.FieldKeyID].(string)
			exists, err := storageProvider.Exists(ctx, builder.secCtx, objID)
			if err != nil {
				t.Errorf("exists %s: %v", objID, err)
			}
			if !exists {
				t.Errorf("object %s not found after create", objID)
			}
		}
	}
}

// TestCacheCheckerTiming_CorrectBehavior tests that cache checker remains set
// until all validations complete (correct behavior)
// Do not use t.Parallel(): storage.SetCacheChecker is process-global.
func TestCacheCheckerTiming_CorrectBehavior(t *testing.T) {
	builder, storageProvider := newTimingTestScenarioBuilder(t)

	// Create test objects with dependencies
	testObjects := []map[string]any{
		{
			objects.FieldKeyKind:     "account",
			objects.FieldKeyID:       "ACC-1785920548450214015-3df55bd1",
			objects.FieldKeyTitle:    "Test User",
			objects.FieldKeyUsername: "test-user",
			objects.FieldKeyStatus:   scenarioBuilderStatusActive,
		},
		{
			objects.FieldKeyKind:       "workstream",
			objects.FieldKeyID:         "WS-TEST-001",
			objects.FieldKeyTitle:      "Test Workstream",
			objects.FieldKeyStatus:     scenarioBuilderStatusActive,
			objects.FieldKeyEntryPoint: "main", // Required field
			objects.FieldKeyOwnerRef:   "ACC-1785920548450214015-3df55bd1",
		},
	}

	// Pre-populate reference cache
	referenceCache := make(map[string]bool)
	referenceCacheMu := &sync.Mutex{}
	idStream := make(map[string]string)
	idStreamMu := &sync.Mutex{}

	for _, obj := range testObjects {
		if objID, ok := obj[objects.FieldKeyID].(string); ok && objID != emptyValue {
			referenceCacheMu.Lock()
			referenceCache[objID] = true
			referenceCacheMu.Unlock()
		}
		if kind, ok := obj[objects.FieldKeyKind].(string); ok && kind == "account" {
			if username, ok := obj[objects.FieldKeyUsername].(string); ok && username != emptyValue {
				accountID := "" // ACC-* from storage;
				referenceCacheMu.Lock()
				referenceCache[accountID] = true
				referenceCacheMu.Unlock()
			}
		}
	}

	// Set cache checker
	storagepkg.SetCacheChecker(func(objectID string) (string, bool) {
		idStreamMu.Lock()
		if _, exists := idStream[objectID]; exists {
			idStreamMu.Unlock()
			return "", true
		}
		idStreamMu.Unlock()

		referenceCacheMu.Lock()
		exists, cached := referenceCache[objectID]
		referenceCacheMu.Unlock()
		if cached && exists {
			return "", true
		}

		return "", false
	})

	// Sequential creates: parallel dual-Create on dependent account/workstream can wedge wg.Wait
	// (SCH-run-cmd-zqk-utility-0). Order preserves CAS visibility for owner_ref.
	ctx := pkgctx.NewSystemContext()
	for _, obj := range testObjects {
		kind, _ := obj[objects.FieldKeyKind].(string)
		objID, _ := obj[objects.FieldKeyID].(string)
		if err := builder.prepareObjectFromDataFile(obj, kind); err != nil {
			t.Fatalf("prepare %s: %v", kind, err)
		}
		if err := builder.validateObjectBeforeCreation(obj, kind); err != nil {
			t.Fatalf("validate %s: %v", kind, err)
		}
		createCtx := storagepkg.WithSyncCreateForKind(pkgctx.WithCacheUpdate(ctx, objID, kind, ""), kind)
		if err := storageProvider.Create(createCtx, builder.secCtx, obj); err != nil {
			t.Fatalf("create %s %s: %v", kind, objID, err)
		}
		idStreamMu.Lock()
		idStream[objID] = objID
		idStreamMu.Unlock()
	}

	// Correct behavior: clear cache checker only after all creates complete
	storagepkg.SetCacheChecker(nil)

	// Verify all objects were created
	for _, obj := range testObjects {
		objID, _ := obj[objects.FieldKeyID].(string)
		exists, err := storageProvider.Exists(ctx, builder.secCtx, objID)
		if err != nil {
			t.Errorf("Failed to check if object %s exists: %v", objID, err)
		}
		if !exists {
			t.Errorf("Object %s was not created", objID)
		}
	}
}
