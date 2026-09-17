package agentclaim

import (
	"context"
	"regexp"
	"strings"

	"github.com/lanceman/zqk/pkg/execwrap"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// gitOutput runs git in root. Tests replace this to avoid a real worktree.
var gitOutput = func(root string, args ...string) (string, error) {
	cmd := execwrap.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

var claimTargetIDPattern = regexp.MustCompile(`\b(?:BLI|PRI)-[A-Za-z0-9-]+`)

// stampClaimProvenance records branch_ref and base_sha on the claimed BLI and
// its priority plan. Empty ProjectRoot skips (same as the check-in timer).
// Already-set fields are left alone so a re-claim cannot move the provenance.
// TRACK: BLI-CEF-R20-BRANCH-REF-SPEC-001
func stampClaimProvenance(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, task map[string]any, opts []ClaimOptions) {
	if sp == nil || len(opts) == 0 || strings.TrimSpace(opts[0].ProjectRoot) == "" {
		return
	}
	root := strings.TrimSpace(opts[0].ProjectRoot)
	branch, err := gitOutput(root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || branch == "" || branch == "HEAD" {
		return
	}
	sha, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil || sha == "" {
		return
	}

	for _, id := range claimTargetIDs(task, opts[0]) {
		stampProvenanceOn(ctx, sp, sec, id, branch, sha)
	}
}

func stampProvenanceOn(ctx context.Context, sp storage.ObjectStorageProvider, sec *pkgctx.SecurityContext, id, branch, sha string) {
	obj, err := sp.Read(ctx, sec, id)
	if err != nil || obj == nil {
		return
	}
	updates := map[string]any{}
	existingBranch := strings.TrimSpace(objects.StringField(obj, objects.FieldKeyBranchName))
	if existingBranch == "" {
		updates[objects.FieldKeyBranchName] = branch
	}
	if strings.TrimSpace(objects.StringField(obj, objects.FieldKeyBaseSha)) == "" {
		updates[objects.FieldKeyBaseSha] = sha
	}
	if len(updates) > 0 {
		_ = sp.Update(ctx, sec, id, updates)
	}
	if kind, _ := obj[objects.FieldKeyKind].(string); kind == objects.KindBacklogItem {
		if pri := strings.TrimSpace(objects.StringField(obj, objects.FieldKeyPriorityPlanRef)); pri != "" {
			stampProvenanceOn(ctx, sp, sec, pri, branch, sha)
		}
	}
}

func claimTargetIDs(task map[string]any, opt ClaimOptions) []string {
	seen := map[string]struct{}{}
	var ids []string
	add := func(raw string) {
		for _, id := range claimTargetIDPattern.FindAllString(raw, -1) {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	add(opt.ForRef)
	if task == nil {
		return ids
	}
	add(objects.StringField(task, "for_ref"))
	for _, key := range []string{objects.FieldKeyRelatedObjectRefs, objects.FieldKeyBacklogItemRefs} {
		switch v := task[key].(type) {
		case []string:
			for _, s := range v {
				add(s)
			}
		case []any:
			for _, item := range v {
				s, _ := item.(string)
				add(s)
			}
		}
	}
	add(objects.StringField(task, objects.FieldKeyTitle))
	add(objects.StringField(task, objects.FieldKeyDescription))
	return ids
}
