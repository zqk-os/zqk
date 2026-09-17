package inbox

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// IngestionParams holds the parameters for ingesting a new agent instruction proposal.
type IngestionParams struct {
	Instruction   string
	TargetPersona string
	SourceSession string
	CreatedBy     string
}

// IngestProposal creates a new agent_instruction proposal in the inbox.
func IngestProposal(ctx context.Context, secCtx *pkgctx.SecurityContext, store storage.ObjectStorageProvider, params IngestionParams) (map[string]any, error) {
	if params.Instruction == "" {
		return nil, errfmt.Errorf("instruction cannot be empty")
	}

	obj := make(map[string]any)
	obj[objects.FieldKeyKind] = "agent_instruction"
	obj[objects.FieldKeyStatus] = "proposed"
	obj[objects.FieldKeyInstruction] = params.Instruction

	if params.TargetPersona != "" {
		obj[objects.FieldKeyTargetPersona] = params.TargetPersona
	}
	if params.SourceSession != "" {
		obj[objects.FieldKeySourceSession] = params.SourceSession
	}

	now := time.Now().UTC().Format(time.RFC3339)
	obj[objects.FieldKeyCreatedAt] = now
	obj[objects.FieldKeyUpdatedAt] = now

	if params.CreatedBy != "" {
		obj[objects.FieldKeyCreatedBy] = params.CreatedBy
		obj[objects.FieldKeyUpdatedBy] = params.CreatedBy
	} else {
		obj[objects.FieldKeyCreatedBy] = objects.DefaultSystemAccountID
		obj[objects.FieldKeyUpdatedBy] = objects.DefaultSystemAccountID
	}

	// Create in storage
	err := store.Create(ctx, secCtx, obj)
	if err != nil {
		return nil, errfmt.Newf("failed to persist agent_instruction").Wrap(err)
	}

	return obj, nil
}
