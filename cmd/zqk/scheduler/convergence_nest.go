package scheduler

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/convergerollup"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

func cvsNestNodeLoader(cmd *cobra.Command) (convergerollup.CVSNodeLoader, *cli.Processor, error) {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, nil, err
	}
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()
	nodeFor := func(id string) ([]string, string, string, error) {
		obj, rerr := proc.Storage().Read(ctx, sec, id)
		if rerr != nil {
			return nil, "", "", rerr
		}
		st, _ := obj[objects.FieldKeyStatus].(string)
		phase, _ := obj[objects.FieldKeyCurrentPhase].(string)
		return cvsIDsFromRelatedObjectRefs(obj[objects.FieldKeyRelatedObjectRefs]), st, phase, nil
	}
	return nodeFor, proc, nil
}

// RunConvergenceNestStatus executes nest-status
func RunConvergenceNestStatus(cmd *cobra.Command, _ []string) error {
	parentID, _ := cmd.Flags().GetString("parent-session-id")
	maxDepth, _ := cmd.Flags().GetInt("max-depth")
	nodeFor, _, err := cvsNestNodeLoader(cmd)
	if err != nil {
		return err
	}
	res, err := convergerollup.NestStatus(parentID, maxDepth, nodeFor)
	if err != nil {
		return err
	}
	return cli.FormatOutput(cmd, res)
}

// RunConvergenceNestLink executes nest-link
func RunConvergenceNestLink(cmd *cobra.Command, _ []string) error {
	parentID, _ := cmd.Flags().GetString("parent-session-id")
	childID, _ := cmd.Flags().GetString("child-session-id")
	maxDepth, _ := cmd.Flags().GetInt("max-depth")
	coordID := parentID
	if f := cmd.Flags().Lookup("coordinator-session-id"); f != nil {
		if v, _ := cmd.Flags().GetString("coordinator-session-id"); strings.TrimSpace(v) != "" {
			coordID = v
		}
	}
	nodeFor, proc, err := cvsNestNodeLoader(cmd)
	if err != nil {
		return err
	}
	if err := convergerollup.ValidateNestLink(coordID, parentID, childID, maxDepth, nodeFor); err != nil {
		return err
	}
	ctx := proc.OperationContext()
	sec := proc.SecurityContext()
	refs := convergerollup.AppendRelatedObjectRef(nil, childID)
	parent, err := proc.Storage().Read(ctx, sec, parentID)
	if err != nil {
		return errfmt.Errorf("read parent %s: %w", parentID, err)
	}
	refs = convergerollup.AppendRelatedObjectRef(convergerollup.RelatedObjectRefsFromMap(parent), childID)
	if err := proc.Storage().Update(ctx, sec, parentID, map[string]any{
		objects.FieldKeyRelatedObjectRefs: refs,
	}); err != nil {
		return errfmt.Errorf("update parent related_object_refs: %w", err)
	}
	return cli.FormatOutput(cmd, map[string]any{
		"parent_id":                       parentID,
		"child_id":                        childID,
		"linked":                          true,
		objects.FieldKeyRelatedObjectRefs: refs,
	})
}

// RunConvergenceNestSpawn executes nest-spawn
func RunConvergenceNestSpawn(cmd *cobra.Command, _ []string) error {
	parentID, _ := cmd.Flags().GetString("parent-session-id")
	title, _ := cmd.Flags().GetString("title")
	hypothesis, _ := cmd.Flags().GetString("hypothesis")
	desired, _ := cmd.Flags().GetString("desired-end-state")
	nextAction, _ := cmd.Flags().GetString("next-action")
	phase, _ := cmd.Flags().GetString("current-phase")
	maxDepth, _ := cmd.Flags().GetInt("max-depth")
	coordID := parentID
	if strings.TrimSpace(title) == "" || strings.TrimSpace(hypothesis) == "" || strings.TrimSpace(desired) == "" {
		return errfmt.Errorf("nest-spawn requires --title, --hypothesis, and --desired-end-state")
	}
	nodeFor, proc, err := cvsNestNodeLoader(cmd)
	if err != nil {
		return err
	}
	placeholder := "CVS-nest-spawn-placeholder"
	if err := convergerollup.ValidateNestLink(coordID, parentID, placeholder, maxDepth, func(id string) ([]string, string, string, error) {
		if id == placeholder {
			return nil, "active", "", nil
		}
		return nodeFor(id)
	}); err != nil {
		return err
	}

	ctx := proc.OperationContext()
	sec := proc.SecurityContext()
	fields := convergerollup.ChildSessionFields(convergerollup.NestSpawnRequest{
		ParentID:        parentID,
		Title:           title,
		Hypothesis:      hypothesis,
		DesiredEndState: desired,
		NextAction:      nextAction,
		CurrentPhase:    phase,
	})
	if err := proc.Storage().Create(ctx, sec, fields); err != nil {
		return errfmt.Errorf("create child convergence_session: %w", err)
	}
	childID, _ := fields[objects.FieldKeyID].(string)
	if childID == "" {
		return errfmt.Errorf("create child returned empty id")
	}
	parent, err := proc.Storage().Read(ctx, sec, parentID)
	if err != nil {
		return errfmt.Errorf("read parent %s: %w", parentID, err)
	}
	refs := convergerollup.AppendRelatedObjectRef(convergerollup.RelatedObjectRefsFromMap(parent), childID)
	if err := proc.Storage().Update(ctx, sec, parentID, map[string]any{
		objects.FieldKeyRelatedObjectRefs: refs,
	}); err != nil {
		return errfmt.Errorf("link child on parent: %w", err)
	}
	return cli.FormatOutput(cmd, map[string]any{
		"parent_id":                       parentID,
		"child_id":                        childID,
		"spawned":                         true,
		objects.FieldKeyRelatedObjectRefs: refs,
		"child":                           fields,
	})
}
