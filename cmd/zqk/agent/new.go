package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewAgentNewCmd creates the ` + "`" + `zqk agent new` + "`" + ` command
func NewAgentNewCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentNewCommandBuilder()
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		personaName := args[0]
		description, _ := cmd.Flags().GetString("description")
		return runAgentNew(cmd, personaName, description)
	}

	return cmd
}

func runAgentNew(cmd *cobra.Command, personaName, description string) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return err
	}

	ctx := proc.OperationContext()
	secCtx := proc.SecurityContext()
	sp := proc.Storage()

	tx, err := sp.BeginTransaction(ctx)
	if err != nil {
		return errfmt.Newf("failed to begin transaction").Wrap(err)
	}
	defer tx.Rollback(ctx)

	ts := time.Now().UnixNano()

	skillID := fmt.Sprintf("ASK-%d", ts)

	// 1. Create Persona
	personaID := fmt.Sprintf("PER-%d", ts)
	if len(strings.TrimSpace(description)) < 10 {
		description = fmt.Sprintf("Operating agent persona for %s.", personaName)
	}
	personaObj := map[string]any{
		objects.FieldKeyKind:           "persona",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyID:             personaID,
		objects.FieldKeyTitle:          personaName,
		objects.FieldKeyName:           personaName,
		objects.FieldKeyRole:           "agent",
		objects.FieldKeyDescription:    description,
		objects.FieldKeyStatus:         objects.ObjectStatusProposed,
		objects.FieldKeyAgentSkillRefs: []any{skillID},
	}
	if err := tx.Create(ctx, secCtx, personaObj); err != nil {
		return errfmt.Newf("failed to create persona").Wrap(err)
	}

	// 2. Create Agent Skill placeholder
	//
	// This command used to also create an assessment_rating (ASR-<ts>) here. That kind's spec
	// was dropped as non-kernel bleedthrough by 9e547bd8c4, so no spec or id-prefix entry
	// defines "ASR": the object persisted but could not be read back ("could not infer kind
	// from ID"), which failed the whole transaction and made `agent new` unusable. Nothing was
	// lost by removing it — .zqk/process/assessment_ratings/ holds zero instances and no object
	// references an ASR id, because the command could never complete. Reintroducing ratings
	// means restoring the spec and prefix first.
	skillObj := map[string]any{
		objects.FieldKeyKind:          "agent_skill",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyID:            skillID,
		objects.FieldKeyTitle:         fmt.Sprintf("Core Skillset for %s", personaName),
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeyProvider:      "zqk",
		objects.FieldKeyInstructions:  "TODO: Add instructions here.",
	}
	if err := tx.Create(ctx, secCtx, skillObj); err != nil {
		return errfmt.Newf("failed to create agent_skill").Wrap(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return errfmt.Newf("failed to commit transaction").Wrap(err)
	}

	// Leave proposed via a manual one-hop (proposed → approved). A raw status
	// Update still validates the lifecycle *edge*, but object promote also
	// refuses auto-only and wildcard hops (* → archived). Require the same
	// neighbor set here so this cannot skip Promote's hop checker.
	if err := promoteStatusOneHop(ctx, secCtx, sp, skillID, objects.ObjectStatusApproved); err != nil {
		return errfmt.Newf("promote agent_skill %s", skillID).Wrap(err)
	}
	if err := promoteStatusOneHop(ctx, secCtx, sp, personaID, objects.ObjectStatusApproved); err != nil {
		return errfmt.Newf("promote persona %s", personaID).Wrap(err)
	}

	return cli.WriteOutput(cmd, []byte(fmt.Sprintf(
		"\n✨ Successfully initialized agent: %s\n   Persona ID: %s\n   Skill Placeholder ID: %s\n\n",
		personaName, personaID, skillID,
	)))
}

// promoteStatusOneHop writes status only when toStatus is a manual promote
// neighbor of the object's current status (objects.PromoteTransitionTargets).
// Storage.Update still enforces IsValidTransition + YAML preconditions.
func promoteStatusOneHop(ctx context.Context, secCtx *pkgctx.SecurityContext, sp storage.ObjectStorageProvider, id, toStatus string) error {
	obj, err := sp.Read(ctx, secCtx, id)
	if err != nil {
		return err
	}
	kind := objects.GetString(obj, objects.FieldKeyKind)
	from := objects.GetString(obj, objects.FieldKeyStatus)
	if err := rejectNonManualPromoteHop(kind, from, toStatus); err != nil {
		return err
	}
	return sp.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyStatus: toStatus})
}

func rejectNonManualPromoteHop(kind, from, to string) error {
	if from == to {
		return nil
	}
	loader := objects.GetGlobalLifecycleLoader()
	lc, err := loader.LoadLifecycle(kind)
	if err != nil {
		return errfmt.Newf("load lifecycle for %s", kind).Wrap(err)
	}
	if _, ok := objects.PromoteTransitionTargets(lc, from)[to]; !ok {
		return errfmt.Errorf("%s: %s → %s is not a manual one-hop promote", kind, from, to)
	}
	var meta objects.Status
	for _, st := range lc.Statuses {
		if st.Value == to {
			meta = st
			break
		}
	}
	// object promote skips archive/error/parking even when the graph lists them
	// (wildcard * → archived is a legal Update edge).
	if objects.IsNonProgressLifecycleStatus(to, meta) {
		return errfmt.Errorf("%s: %s → %s is not a progress promote hop", kind, from, to)
	}
	return nil
}
