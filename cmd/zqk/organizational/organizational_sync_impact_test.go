package organizational

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type interceptingOrgStorage struct {
	storage.ObjectStorageProvider
	readHook   func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	updateHook func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
	existsHook func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error)
	createHook func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
}

func (s *interceptingOrgStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if s.readHook != nil {
		return s.readHook(ctx, secCtx, id)
	}
	return s.ObjectStorageProvider.Read(ctx, secCtx, id)
}

func (s *interceptingOrgStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if s.updateHook != nil {
		return s.updateHook(ctx, secCtx, id, updates)
	}
	return s.ObjectStorageProvider.Update(ctx, secCtx, id, updates)
}

func (s *interceptingOrgStorage) Exists(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (bool, error) {
	if s.existsHook != nil {
		return s.existsHook(ctx, secCtx, id)
	}
	return s.ObjectStorageProvider.Exists(ctx, secCtx, id)
}

func (s *interceptingOrgStorage) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	if s.createHook != nil {
		return s.createHook(ctx, secCtx, obj)
	}
	return s.ObjectStorageProvider.Create(ctx, secCtx, obj)
}

func (s *interceptingOrgStorage) GetProjectRoot() string {
	if fos, ok := s.ObjectStorageProvider.(interface{ GetProjectRoot() string }); ok {
		return fos.GetProjectRoot()
	}
	return ""
}

func setupElevationWorkspace(t *testing.T, scenario string) (string, *storage.FileObjectStorage) {
	t.Helper()
	testRoot, sp, cleanupFn := setupOrgCliWorkspace(t)
	require.NotNil(t, cleanupFn)
	fos, ok := sp.(*storage.FileObjectStorage)
	require.True(t, ok)
	return testRoot, fos
}

func runElevationCmd(t *testing.T, cmd *cobra.Command, root string, sp storage.ObjectStorageProvider, args []string) (string, error) {
	t.Helper()
	cmd.SetArgs(args)
	ctx := pkgctx.NewSystemContext()
	ctx = cli.WithStorageProvider(ctx, sp)
	cmd.SetContext(ctx)
	cli.SetContext(cmd, cli.ContextForProjectRoot(root))

	var execErr error
	out := captureStdoutCliOrg(t, func() {
		execErr = cmd.Execute()
	})
	return out, execErr
}

func TestElevation_AnalyzeImpact_Coverage(t *testing.T) {
	testRoot, store := setupElevationWorkspace(t, "analyze-elevation")

	// 1. Non-existent change error in AnalyzeChange
	cmdMissing := NewAnalyzeImpactCmd()
	_, errMissing := runElevationCmd(t, cmdMissing, testRoot, store, []string{"--change", "OCH-999"})
	assert.Error(t, errMissing)

	// 2. Storage read error fallback formatting
	changeID := "OCH-101"
	recordCmd := NewRecordChangeCmd()
	_, recErr := runElevationCmd(t, recordCmd, testRoot, store, []string{
		"--id", changeID,
		"--change-type", "restructure",
		"--title", "Restructure Read Fail Test",
	})
	require.NoError(t, recErr)

	wrapper := &interceptingOrgStorage{
		ObjectStorageProvider: store,
		readHook: func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			if strings.HasPrefix(id, "IMP-") {
				return nil, fmt.Errorf("simulated read failure on impact analysis")
			}
			return store.Read(ctx, secCtx, id)
		},
	}
	cmdFallback := NewAnalyzeImpactCmd()
	outFallback, errFallback := runElevationCmd(t, cmdFallback, testRoot, wrapper, []string{"--change", changeID})
	require.NoError(t, errFallback)
	assert.Contains(t, outFallback, "Impact analysis created: IMP-")
}

func TestElevation_GetOrCreateImpactAnalysis_Branches(t *testing.T) {
	testRoot, store := setupElevationWorkspace(t, "get-or-create-impact")
	require.NotEmpty(t, testRoot)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Branch 1: direct impact_analysis_refs on change object
	refObj := map[string]any{
		objects.FieldKeyImpactAnalysisRefs: []any{"IMP-101"},
	}
	id1 := getOrCreateImpactAnalysis(ctx, store, secCtx, refObj, "OCH-101", logger)
	assert.Equal(t, "IMP-101", id1)

	// Branch 2: found via store.List by change_ref
	storedImpactID := "IMP-102"
	targetChangeID := "OCH-102"
	createErr := store.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            storedImpactID,
		objects.FieldKeyKind:          objects.KindImpactAnalysis,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Stored Impact Test",
		objects.FieldKeyDescription:   "Stored impact description",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeyChangeRef:     targetChangeID,
		objects.FieldKeyChangeType:    "restructure",
	})
	require.NoError(t, createErr)
	id2 := getOrCreateImpactAnalysis(ctx, store, secCtx, map[string]any{}, targetChangeID, logger)
	assert.Equal(t, storedImpactID, id2)

	// Branch 3: found via draft plane lookup
	draftImpactID := "IMP-103"
	draftChangeID := "OCH-103"
	draftData := []byte(fmt.Sprintf(
		"id: %s\nkind: %s\nchange_ref: %s\nschema_version: %q\nstatus: proposed\ntitle: Draft Impact\n",
		draftImpactID, objects.KindImpactAnalysis, draftChangeID, objects.DefaultSchemaVersion,
	))
	require.NoError(t, store.WriteObjectToDraftPlane(draftImpactID, objects.KindImpactAnalysis, draftData))
	id3 := getOrCreateImpactAnalysis(ctx, store, secCtx, map[string]any{}, draftChangeID, logger)
	assert.Equal(t, draftImpactID, id3)

	// Branch 4: analyzer generates new impact analysis
	dynamicChangeID := "OCH-104"
	dynChangeErr := store.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            dynamicChangeID,
		objects.FieldKeyKind:          objects.KindOrganizationalChange,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Dynamic Org Change",
		objects.FieldKeyDescription:   "Dynamic change description",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeyChangeType:    "team_reassignment",
	})
	require.NoError(t, dynChangeErr)
	id4 := getOrCreateImpactAnalysis(ctx, store, secCtx, map[string]any{}, dynamicChangeID, logger)
	assert.NotEmpty(t, id4)
	assert.True(t, strings.HasPrefix(id4, "IMP-"))

	// Branch 5: analyzer error returns empty string
	id5 := getOrCreateImpactAnalysis(ctx, store, secCtx, map[string]any{}, "OCH-998", logger)
	assert.Empty(t, id5)
}

func TestElevation_Propagate_ResolutionErrors(t *testing.T) {
	testRoot, store := setupElevationWorkspace(t, "propagate-res-err")
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. impactAnalysisID == "" error path
	brokenChangeID := "OCH-201"
	createErr := store.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            brokenChangeID,
		objects.FieldKeyKind:          objects.KindOrganizationalChange,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Change With Corrupt Analyzer",
		objects.FieldKeyDescription:   "Change description",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeyChangeType:    "team_reassignment",
	})
	require.NoError(t, createErr)

	failingStore := &interceptingOrgStorage{
		ObjectStorageProvider: store,
		createHook: func(c context.Context, s *pkgctx.SecurityContext, obj map[string]any) error {
			kindVal, ok := obj[objects.FieldKeyKind]
			if ok && kindVal == objects.KindImpactAnalysis {
				return fmt.Errorf("simulated impact analysis creation failure")
			}
			return store.Create(c, s, obj)
		},
	}
	cmdNoImpact := NewPropagateCmd()
	_, errNoImpact := runElevationCmd(t, cmdNoImpact, testRoot, failingStore, []string{"--change", brokenChangeID})
	assert.Error(t, errNoImpact)
	assert.Contains(t, errNoImpact.Error(), "no impact analysis for change")

	// 2. store.Read error on impact analysis object
	cmdReadErr := NewPropagateCmd()
	_, errReadImpact := runElevationCmd(t, cmdReadErr, testRoot, store, []string{
		"--change", brokenChangeID,
		"--impact-analysis", "IMP-999",
	})
	assert.Error(t, errReadImpact)
	assert.Contains(t, errReadImpact.Error(), "failed to read impact analysis")
}

func TestElevation_Propagate_ConfirmModeUpdateHandling(t *testing.T) {
	testRoot, store := setupElevationWorkspace(t, "propagate-confirm")
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	changeValidID := "OCH-202"
	require.NoError(t, store.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            changeValidID,
		objects.FieldKeyKind:          objects.KindOrganizationalChange,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Confirm Propagate Change",
		objects.FieldKeyDescription:   "Description",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeyChangeType:    "restructure",
	}))

	impactValidID := "IMP-202"
	targetBliID := "BLI-201"
	errTargetBliID := "BLI-202"
	require.NoError(t, store.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            impactValidID,
		objects.FieldKeyKind:          objects.KindImpactAnalysis,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Impact for Confirm Test",
		objects.FieldKeyDescription:   "Description",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeyChangeRef:     changeValidID,
		objects.FieldKeyAffectedObjects: map[string]any{
			"backlog_items": []any{12345, "", targetBliID, errTargetBliID},
		},
	}))
	require.NoError(t, store.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:            targetBliID,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Target BLI",
		objects.FieldKeyDescription:   "Target BLI description",
		objects.FieldKeyStatus:        objects.ObjectStatusValidated,
	}))

	updateFailStore := &interceptingOrgStorage{
		ObjectStorageProvider: store,
		updateHook: func(c context.Context, s *pkgctx.SecurityContext, id string, updates map[string]any) error {
			if id == errTargetBliID {
				return fmt.Errorf("simulated update failure on object %s", id)
			}
			if id == targetBliID {
				return nil
			}
			return store.Update(c, s, id, updates)
		},
	}
	cmdConfirm := NewPropagateCmd()
	outConfirm, errConfirm := runElevationCmd(t, cmdConfirm, testRoot, updateFailStore, []string{
		"--change", changeValidID,
		"--impact-analysis", impactValidID,
		"--confirm",
	})
	require.NoError(t, errConfirm)
	assert.Contains(t, outConfirm, "1 objects updated with last_propagated_change_ref")
	assert.Contains(t, outConfirm, "Change propagation complete")
}

func TestElevation_Sync_ContinueOnErrorAndValidation(t *testing.T) {
	testRoot, store := setupElevationWorkspace(t, "sync-validation-paths")

	// 1. Missing kind with continue-on-error=true
	noKindYaml := filepath.Join(testRoot, "nokind_continue.yaml")
	require.NoError(t, fileutil.WriteFile(noKindYaml, []byte("- title: No Kind Row\n"), paths.FilePerm600))
	cmd1 := NewSyncCmd()
	out1, err1 := runElevationCmd(t, cmd1, testRoot, store, []string{
		"--file", noKindYaml,
		"--continue-on-error",
	})
	require.NoError(t, err1)
	assert.Empty(t, out1)

	// 2. Disallowed kind with continue-on-error=true
	disallowedYaml := filepath.Join(testRoot, "disallowed_continue.yaml")
	disallowedContent := "- id: BLI-301\n  kind: backlog_item\n  title: Skip Me\n"
	require.NoError(t, fileutil.WriteFile(disallowedYaml, []byte(disallowedContent), paths.FilePerm600))
	cmd2 := NewSyncCmd()
	out2, err2 := runElevationCmd(t, cmd2, testRoot, store, []string{
		"--file", disallowedYaml,
		"--continue-on-error",
	})
	require.NoError(t, err2)
	assert.Empty(t, out2)

	// 3. Valid org object with mode=create_only
	createOnlyYaml := filepath.Join(testRoot, "create_only.yaml")
	createOnlyContent := `
- id: ORG-301
  kind: organization
  namespace_id: domain:organizational
  schema_version: "` + objects.DefaultSchemaVersion + `"
  title: Create Only Org
  status: active
  domain: custom
  spec_interpreter: default
  spec_context_broker: default
  organization_name: Create Only Org
  division_refs: []
`
	require.NoError(t, fileutil.WriteFile(createOnlyYaml, []byte(createOnlyContent), paths.FilePerm600))
	cmd3 := NewSyncCmd()
	_, err3 := runElevationCmd(t, cmd3, testRoot, store, []string{
		"--file", createOnlyYaml,
		"--mode", "create_only",
	})
	if err3 != nil {
		assert.Contains(t, err3.Error(), "no builders found for ontology")
	}
}

func TestElevation_Sync_ImportErrorAndReporting(t *testing.T) {
	testRoot, store := setupElevationWorkspace(t, "sync-error-reporting")

	// 1. Missing ID under upsert mode causes import errors logged via continue-on-error
	missingIDYaml := filepath.Join(testRoot, "missing_id_upsert.yaml")
	missingIDContent := `
- kind: organization
  namespace_id: domain:organizational
  schema_version: "` + objects.DefaultSchemaVersion + `"
  title: Org Missing ID
  status: active
  domain: custom
  spec_interpreter: default
  spec_context_broker: default
  organization_name: Org Missing ID
  division_refs: []
`
	require.NoError(t, fileutil.WriteFile(missingIDYaml, []byte(missingIDContent), paths.FilePerm600))
	cmdMissingID := NewSyncCmd()
	outMissingID, errMissingID := runElevationCmd(t, cmdMissingID, testRoot, store, []string{
		"--file", missingIDYaml,
		"--continue-on-error",
	})
	require.NoError(t, errMissingID)
	assert.Empty(t, outMissingID)

	// 2. Storage provider failure on Exists during import
	failingStore := &interceptingOrgStorage{
		ObjectStorageProvider: store,
		existsHook: func(c context.Context, s *pkgctx.SecurityContext, id string) (bool, error) {
			return false, fmt.Errorf("simulated storage exists error")
		},
	}
	validYaml := filepath.Join(testRoot, "valid_fail_exists.yaml")
	validContent := `
- id: ORG-302
  kind: organization
  namespace_id: domain:organizational
  schema_version: "` + objects.DefaultSchemaVersion + `"
  title: Exists Fail Org
  status: active
  domain: custom
  spec_interpreter: default
  spec_context_broker: default
  organization_name: Exists Fail Org
  division_refs: []
`
	require.NoError(t, fileutil.WriteFile(validYaml, []byte(validContent), paths.FilePerm600))
	cmdFailExists := NewSyncCmd()
	_, errFailExists := runElevationCmd(t, cmdFailExists, testRoot, failingStore, []string{
		"--file", validYaml,
	})
	assert.Error(t, errFailExists)
	assert.Contains(t, errFailExists.Error(), "organizational sync failed")
}

func TestElevation_Sync_MarshalUnmarshalFormats(t *testing.T) {
	t.Parallel()

	// 1. Single JSON map unmarshal fallback
	singleJSON := `{"id":"ORG-303","kind":"organization","title":"Single JSON Org"}`
	singleJSONObjs, errJSON := unmarshalOrgSync([]byte(singleJSON), storage.ExportFormatJSON)
	require.NoError(t, errJSON)
	assert.Len(t, singleJSONObjs, 1)
	assert.Equal(t, "ORG-303", singleJSONObjs[0]["id"])

	// 2. Invalid JSON unmarshal error
	_, errInvalidJSON := unmarshalOrgSync([]byte("{invalid json"), storage.ExportFormatJSON)
	assert.Error(t, errInvalidJSON)

	// 3. Invalid YAML unmarshal error
	_, errInvalidYAML := unmarshalOrgSync([]byte(":\n  invalid:\n    - :"), storage.ExportFormatYAML)
	assert.Error(t, errInvalidYAML)

	// 4. Unsupported unmarshal format
	_, errUnsupportedUnmarshal := unmarshalOrgSync([]byte("test"), storage.ExportFormat("toml"))
	assert.Error(t, errUnsupportedUnmarshal)
	assert.Contains(t, errUnsupportedUnmarshal.Error(), "unsupported format: toml")

	// 5. Marshal empty format defaults to YAML
	dataEmptyFmt, errEmptyFmt := marshalOrgSync(singleJSONObjs, "")
	require.NoError(t, errEmptyFmt)
	assert.Contains(t, string(dataEmptyFmt), "ORG-303")

	// 6. Marshal unsupported format returns error
	_, errUnsupportedMarshal := marshalOrgSync(singleJSONObjs, storage.ExportFormat("xml"))
	assert.Error(t, errUnsupportedMarshal)
	assert.Contains(t, errUnsupportedMarshal.Error(), "unsupported format: xml")
}
