package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
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
	// means restoring the spec and prefix first. TRACK: BLI-1787556517612216000-d382f41e
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

	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	promote := func(id, leaveStatus string) error {
		return sp.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyStatus: leaveStatus})
	}
	// Approved, not implemented: persona_lifecycle.yaml only allows
	// proposed -> approved -> in_progress -> implemented, so promoting a freshly drafted
	// persona straight to implemented is an illegal transition and fails the command.
	// Approved is also the right meaning here — the persona is ready to use, not finished —
	// and it matches the agent_skill promote below.
	if err := promote(skillID, objects.ObjectStatusApproved); err != nil {
		return errfmt.Newf("promote agent_skill %s", skillID).Wrap(err)
	}
	if err := promote(personaID, objects.ObjectStatusApproved); err != nil {
		return errfmt.Newf("promote persona %s", personaID).Wrap(err)
	}

	return cli.WriteOutput(cmd, []byte(fmt.Sprintf(
		"\n✨ Successfully initialized agent: %s\n   Persona ID: %s\n   Skill Placeholder ID: %s\n\n",
		personaName, personaID, skillID,
	)))
}
