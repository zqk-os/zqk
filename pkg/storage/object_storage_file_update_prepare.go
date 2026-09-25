package storage

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/crud"
	"github.com/zqk-os/zqk/pkg/when"
)

type fileObjectUpdatePrep struct {
	ctx                     context.Context
	secCtx                  *pkgctx.SecurityContext
	id                      string
	updates                 map[string]any
	kind                    string
	existing                map[string]any
	previousStateForJournal map[string]any
	spec                    *objects.Spec
	newID                   string
	idUpdated               bool
	expectedUpdatedAt       string
	isBuiltIn               bool
	hasAdminRole            bool
	oldState                string
	newState                string
	effectiveUpdates        map[string]any
	runtimeDeltaOnly        bool
}

func (f *FileObjectStorage) prepareFileObjectUpdate(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) (*fileObjectUpdatePrep, error) {
	// Infer kind from ID first (more reliable than reading from file)
	if err := f.idValidator.LoadPatterns(); err != nil {
		return nil, errfmt.Newf(ErrMsgLoadIDPatterns).Wrap(err)
	}
	kind := f.idValidator.InferKindFromID(id)
	if kind == emptyValue {
		return nil, errfmt.Errorf(ErrMsgInferKindFailed, id)
	}

	// For keystore_entry, we need to read with system context to get all fields
	// (including credential_hash which is required for validation)
	// Access control will be applied when the updated object is read later
	var existing map[string]any
	var err error
	keystoreDir := objects.GetDirectoryFromKind(objects.KindKeystoreEntry)
	when.When(func() bool {
		return keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir && secCtx != nil && secCtx.AccountID != pkgctx.SystemAccountID
	}).Then(func() {
		systemCtx := pkgctx.NewSystemSecurityContext()
		existing, err = f.Read(ctx, systemCtx, id)
	}).OrElse(func() {
		existing, err = f.Read(ctx, secCtx, id)
	}).Run()
	if err != nil {
		if !crud.LiveCASBlobUnreadable(err) {
			return nil, err
		}
		// Unparseable live blob (committed conflict markers, truncated YAML):
		// merge-with-current is impossible. Treat the update map as the full replacement
		// when it carries the same id (repair-yaml / --file rewrite).
		updateID, _ := updates[objects.FieldKeyID].(string)
		if strings.TrimSpace(updateID) != id {
			return nil, err
		}
		existing = maps.Clone(updates)
		if existing == nil {
			existing = make(map[string]any)
		}
	}

	// Ensure kind matches inferred kind (not ontology from file)
	existing[objects.FieldKeyKind] = kind

	// Capture previous state for change journal BEFORE merging updates
	previousStateForJournal := make(map[string]any)
	maps.Copy(previousStateForJournal, existing)

	// Check for blocking issues before write operations (except for automated kinds)
	if err := f.checkForBlockingIssuesBeforeWrite(ctx, OpUpdate, kind, id); err != nil {
		return nil, err
	}

	// Check permission
	if err := f.checkPermission(secCtx, "write", kind); err != nil {
		return nil, err
	}

	// For keystore_entry, verify user owns the entry or is admin
	if keystoreDir != emptyValue && objects.GetDirectoryFromKind(kind) == keystoreDir {
		entryAccountID, _ := existing[objects.FieldKeyAccountID].(string)
		isAdmin := slices.Contains(secCtx.Roles, "admin")
		if !isAdmin && entryAccountID != secCtx.AccountID {
			return nil, errfmt.Errorf(ErrMsgPermUpdateKeystore)
		}
		// Prevent non-system users from updating credential_hash or salt
		if secCtx != nil && secCtx.AccountID != pkgctx.SystemAccountID {
			if _, hasHash := updates[objects.FieldKeyCredentialHash]; hasHash {
				return nil, errfmt.Errorf(ErrMsgPermUpdateCredHash)
			}
			if _, hasSalt := updates[objects.FieldKeySalt]; hasSalt {
				return nil, errfmt.Errorf(ErrMsgPermUpdateSalt)
			}
		}
	}

	// Normalize status (kind-aware). Unknown lifecycle values fail closed.
	if s := objects.GetString(updates, objects.FieldKeyStatus); s != emptyValue {
		canonical, canonErr := canonicalizePersistedLifecycleStatus(f.GetLifecycleLoader(), kind, s)
		if canonErr != nil {
			return nil, canonErr
		}
		updates[objects.FieldKeyStatus] = canonical
	}

	if s := objects.GetString(updates, objects.FieldKeyStatus); s != "" {
		if err := checkVerificationOutcomeAuthority(kind, secCtx, s); err != nil {
			return nil, err
		}
	}

	// For occupiable kinds transitioning to in_progress: if claimed_by is present but claimed_at is missing, stamp claimed_at.
	if hasOccupiable, _ := objects.KindHasTrait(kind, objects.TraitOccupiable); hasOccupiable {
		newStatus := objects.GetString(updates, objects.FieldKeyStatus)
		if newStatus == objects.ObjectStatusInProgress {
			claimedBy := objects.GetString(updates, objects.FieldKeyClaimedBy)
			if claimedBy == "" {
				claimedBy = objects.GetString(existing, objects.FieldKeyClaimedBy)
			}
			claimedAt := objects.GetString(updates, objects.FieldKeyClaimedAt)
			if claimedAt == "" {
				claimedAt = objects.GetString(existing, objects.FieldKeyClaimedAt)
			}
			if claimedBy != "" && claimedAt == "" {
				updates[objects.FieldKeyClaimedAt] = time.Now().UTC().Format(time.RFC3339)
			}
		}
	}

	// --- TDE Verification Override Enforcement ---
	if statusVal, ok := updates[objects.FieldKeyStatus].(string); ok && (kind == "test_case" || kind == "criteria" || kind == "convergence_session") {
		if objects.GetGlobalStatusChecker().IsTerminal(kind, statusVal) && statusVal != "archived" {
			isScheduler := false
			if secCtx != nil {
				isScheduler = slices.Contains(secCtx.Roles, "scheduler") || secCtx.AccountID == pkgctx.SystemAccountID
			}

			if !isScheduler {
				switch kind {
				case "criteria":
					updates[objects.FieldKeyStatus] = "awaiting_verification"
				case "convergence_session":
					updates[objects.FieldKeyStatus] = "c1_scope"
				case "test_case":
					// test_case has no verification-hold status (pending_verification is agent_task).
					// Refuse the terminal hop by keeping the current legal status.
					if cur := objects.GetString(existing, objects.FieldKeyStatus); cur != "" {
						updates[objects.FieldKeyStatus] = cur
					} else {
						updates[objects.FieldKeyStatus] = objects.ObjectStatusDraft
					}
				default:
					updates[objects.FieldKeyStatus] = "pending_verification"
				}
				inboxPath := filepath.Join(f.projectRoot, paths.ProjectDataDir, paths.InboxSubdir, "human")
				_ = fileutil.EnsureDir(inboxPath)
				tdePath := filepath.Join(inboxPath, "tde_"+id+"_"+time.Now().Format("20060102150405")+".json")
				tdeData := []byte(`{"id":"` + id + `", "action": "status_override", "requested_status": "` + statusVal + `"}`)
				_ = fileutil.WriteStandardFile(tdePath, tdeData)
			}
		}
	}
	// ---------------------------------------------

	// Optimistic locking: check updated_at if provided in updates
	// Note: We check this early, but for true concurrency safety, we should re-check
	// right before writing. For now, this provides basic optimistic locking.
	var expectedUpdatedAt string
	if _, hasExpected := updates[FieldKeyExpectedUpdatedAt]; hasExpected {
		expectedUpdatedAt = objects.GetString(updates, FieldKeyExpectedUpdatedAt)
		delete(updates, FieldKeyExpectedUpdatedAt)
		if expectedUpdatedAt != "" {
			actualUpdatedAt, _ := existing[objects.FieldKeyUpdatedAt].(string)
			if actualUpdatedAt != expectedUpdatedAt {
				return nil, ErrVersionConflict
			}
		}
	}

	// Check if this is a built-in object and user has admin role
	isBuiltIn := IsBuiltIn(existing)
	hasAdminRole := slices.Contains(secCtx.Roles, "admin")

	// Load spec for the kind to enforce spec-driven update modes
	var spec *objects.Spec
	loader := f.specLoader
	if loader == nil {
		loader = objects.GetGlobalSpecLoader()
	}
	if loader != nil {
		specFile := fmt.Sprintf("%s.yaml", kind)
		var specErr error
		spec, specErr = loader.LoadSpecWithInheritance(specFile)
		if specErr != nil && !IsExpectedMissingErr(specErr) {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

				// Check if ID is being updated
				Error(ErrMsgSwallowedError, specErr).Log()
		}
	}

	var newID string
	var idUpdated bool
	if newIDValue := objects.GetString(updates, objects.FieldKeyID); newIDValue != "" && newIDValue != id {
		newID = newIDValue
		idUpdated = true

		// Validate new ID format
		if err := f.idValidator.LoadPatterns(); err != nil {
			return nil, errfmt.Newf(ErrMsgLoadIDPatterns).Wrap(err)
		}
		valid, err := f.idValidator.ValidateID(newID, kind)
		if err != nil {
			return nil, errfmt.Newf(ErrMsgValidateNewID).Wrap(err)
		}
		if !valid {
			return nil, errfmt.Errorf(ErrMsgInvalidIDFormat, kind, newID)
		}

		// Check if new ID already exists
		_, err = f.Read(ctx, secCtx, newID)
		if err == nil {
			return nil, errfmt.Errorf(ErrMsgObjectExistsFmt, newID)
		}
		// For CAS objects, Read might return different error formats
		errStr := err.Error()
		if !errors.Is(err, ErrObjectNotFound) && !strings.Contains(errStr, "not found") && !strings.Contains(errStr, "ID not found") {
			return nil, errfmt.Newf(ErrMsgCheckNewIDExists).Wrap(err)
		}
	}

	return &fileObjectUpdatePrep{
		ctx:                     ctx,
		secCtx:                  secCtx,
		id:                      id,
		updates:                 updates,
		kind:                    kind,
		existing:                existing,
		previousStateForJournal: previousStateForJournal,
		spec:                    spec,
		newID:                   newID,
		idUpdated:               idUpdated,
		expectedUpdatedAt:       expectedUpdatedAt,
		isBuiltIn:               isBuiltIn,
		hasAdminRole:            hasAdminRole,
	}, nil
}
