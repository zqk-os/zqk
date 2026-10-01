package matrix

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/quality"
)

func runMatrixUpdate(cmd *cobra.Command, _ []string) error {
	ctx := cli.GetContext(cmd)
	projectRoot := ""
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == "" {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == "" {
		return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations("project root not found; run from repo root or zqk use"))
	}

	name, _ := cmd.Flags().GetString("name")
	registryFlag, _ := cmd.Flags().GetString("registry")
	matrixPath, _ := cmd.Flags().GetString("matrix")
	profilePath, _ := cmd.Flags().GetString("profile")
	filePath, _ := cmd.Flags().GetString("file-path")
	bundleLabel, _ := cmd.Flags().GetString("bundle-label")
	filterPairs, _ := cmd.Flags().GetStringArray("filter")
	limit, _ := cmd.Flags().GetInt("limit")
	setPairs, _ := cmd.Flags().GetStringArray("set")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	backup, _ := cmd.Flags().GetBool("backup")
	backupTo, _ := cmd.Flags().GetString("backup-to")
	appendCvs, _ := cmd.Flags().GetBool("append-cvs-activity")
	cvsIDsFlag, _ := cmd.Flags().GetStringArray("cvs-id")
	writeOpts := &quality.MatrixWriteOpts{Backup: backup, BackupPath: strings.TrimSpace(backupTo)}

	if len(cvsIDsFlag) > 0 && !appendCvs {
		return errfmt.Errorf("--cvs-id requires --append-cvs-activity")
	}
	if appendCvs && dryRun {
		return errfmt.Errorf("--append-cvs-activity cannot be used with --dry-run")
	}
	if appendCvs && len(cvsIDsFlag) > 0 {
		if err := validateCvsIDsReadable(cmd, dedupeSortedStrings(cvsIDsFlag)); err != nil {
			return err
		}
	}

	filters, err := quality.ParseColumnValuePairs(filterPairs, false, "--filter")
	if err != nil {
		return err
	}

	fp := strings.TrimSpace(filePath)
	bl := strings.TrimSpace(bundleLabel)
	hasBulk := len(filters) > 0

	if hasBulk && (fp != "" || bl != "") {
		return errfmt.Errorf("do not combine --filter with --file-path or --bundle-label")
	}

	resolved, err := quality.ResolveMatrixForCLI(projectRoot, name, registryFlag, matrixPath, profilePath)
	if err != nil {
		return err
	}

	if hasBulk {
		res, err := quality.UpdateMatrixCSVByFilter(resolved.CSVPath, resolved.ProfilePath, resolved.Alias, filters, setPairs, resolved.ValueMap, limit, dryRun, writeOpts, resolved.SessionRefColumn)
		if err != nil {
			return err
		}
		if appendCvs && res.Wrote {
			ar, err := appendMatrixCvsActivity(cmd, resolved.Alias, resolved.CSVPath, true, res.UpdatedCount, res.Updates, cvsIDsFlag, resolved.SessionRefColumn, nil, res)
			if err != nil {
				return err
			}
			res.CvsActivityAppend = ar
		}
		return cli.FormatOutput(cmd, res)
	}

	if (fp == "") == (bl == "") {
		return errfmt.Errorf("exactly one of --file-path or --bundle-label is required, or use --filter for bulk update")
	}

	matchCol := "file_path"
	matchVal := fp
	if bl != "" {
		matchCol = "bundle_label"
		matchVal = bl
	}

	res, err := quality.UpdateMatrixCSVRow(resolved.CSVPath, resolved.ProfilePath, resolved.Alias, matchCol, matchVal, setPairs, resolved.ValueMap, dryRun, writeOpts)
	if err != nil {
		return err
	}
	if appendCvs && res.Wrote {
		ar, err := appendMatrixCvsActivity(cmd, resolved.Alias, resolved.CSVPath, false, 1, res.Updates, cvsIDsFlag, resolved.SessionRefColumn, res, nil)
		if err != nil {
			return err
		}
		res.CvsActivityAppend = ar
	}
	return cli.FormatOutput(cmd, res)
}

func validateCvsIDsReadable(cmd *cobra.Command, ids []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, ids []string, proc *cli.Processor) error {
		if len(ids) == 0 {
			return nil
		}
		var err error
		_ = err
		ctx := proc.OperationContext()
		sec := proc.SecurityContext()
		for _, id := range ids {
			obj, err := proc.Storage().Read(ctx, sec, id)
			if err != nil {
				return errfmt.Errorf("matrix update: CVS %q not readable before CSV write: %w", id, err)
			}
			kind, _ := obj[objects.FieldKeyKind].(string)
			if kind != objects.KindConvergenceSession {
				return errfmt.Errorf("matrix update: object %q has kind %q, expected %s", id, kind, objects.KindConvergenceSession)
			}
		}
		return nil
	})(cmd, ids)
}

func appendMatrixCvsActivity(cmd *cobra.Command, matrixAlias, csvPath string, bulk bool, updatedCount int, updates map[string]string, cvsIDsFlag []string, sessionCol string, single *quality.MatrixUpdateResult, bulkRes *quality.MatrixBulkUpdateResult) (*quality.MatrixCvsActivityAppendResult, error) {
	ids, err := resolveMatrixCvsIDsForAppend(cvsIDsFlag, sessionCol, single, bulkRes)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errfmt.Errorf("no convergence session ids found for --append-cvs-activity (use --cvs-id or ensure session_ref_column has values on updated rows)")
	}

	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, errfmt.Newf("matrix update: storage for CVS append").Wrap(err)
	}
	logger := logging.GetLoggerFromContext(cmd.Context())

	entry := quality.BuildMatrixActivityLogEntry(matrixAlias, csvPath, bulk, updatedCount, updates)
	payload := map[string]any{
		objects.FieldKeyActivityLog: []any{entry},
	}

	out := &quality.MatrixCvsActivityAppendResult{
		Requested: true,
		IDs:       ids,
		Failed:    make(map[string]string),
	}
	opCtx := proc.OperationContext()
	secCtx := proc.SecurityContext()

	for _, id := range ids {
		if err := proc.Storage().Update(opCtx, secCtx, id, payload); err != nil {
			out.Failed[id] = err.Error()
			logging.FluentEvent(logger).Warn("matrix update: CVS activity append failed").
				ObjectID(id).
				WithError(err).
				Log()
			continue
		}
		out.Succeeded = append(out.Succeeded, id)
		logging.FluentEvent(logger).Info("matrix update: appended convergence_session activity_log").
			ObjectID(id).
			Log()
	}
	if len(out.Succeeded) == 0 {
		return out, errfmt.Errorf("matrix update: CVS activity append failed for all ids (%d error(s))", len(out.Failed))
	}
	if len(out.Failed) > 0 {
		logging.FluentEvent(logger).Warn("matrix update: CVS activity append partial failure").
			Int("succeeded", len(out.Succeeded)).
			Int("failed", len(out.Failed)).
			Log()
	}
	return out, nil
}

func resolveMatrixCvsIDsForAppend(cvsFlag []string, sessionCol string, single *quality.MatrixUpdateResult, bulk *quality.MatrixBulkUpdateResult) ([]string, error) {
	var explicit []string
	for _, id := range cvsFlag {
		id = strings.TrimSpace(id)
		if id != "" {
			explicit = append(explicit, id)
		}
	}
	if len(explicit) > 0 {
		return dedupeSortedStrings(explicit), nil
	}
	sessionCol = strings.TrimSpace(sessionCol)
	if sessionCol == "" {
		return nil, errfmt.Errorf("matrix update: --append-cvs-activity needs --cvs-id or a registry session_ref_column (not available when using --matrix/--profile without session column)")
	}
	if single != nil && single.RowAfter != nil {
		v := strings.TrimSpace(single.RowAfter[sessionCol])
		if v == "" {
			return nil, errfmt.Errorf("matrix update: updated row has empty %q; cannot append CVS activity", sessionCol)
		}
		return []string{v}, nil
	}
	if bulk != nil && len(bulk.CvsIDs) > 0 {
		return bulk.CvsIDs, nil
	}
	return nil, errfmt.Errorf("matrix update: no %q values on updated rows", sessionCol)
}

func dedupeSortedStrings(in []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}
