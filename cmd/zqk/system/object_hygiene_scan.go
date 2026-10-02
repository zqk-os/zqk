package system

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/processhygiene"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewObjectHygieneScanCmd scans objects using pkg/processhygiene rules
// (--rules-file only, else storage process_hygiene_rule rows, else internal YAML, else embedded defaults).
// Command structure and flags: .zqk/cli/specs/system/object_hygiene_scan_command.yaml.
func NewObjectHygieneScanCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemObjectHygieneScanCommandBuilder(), &cobra.Command{Use: "object-hygiene-scan"})
	cli.BindAsyncProgress(cmd, runObjectHygieneScan)
	return cmd
}

type objectHygieneScanResult struct {
	ProjectRoot    string                   `json:"project_root"`
	RulesSource    string                   `json:"rules_source"`
	KindsScanned   int                      `json:"kinds_scanned"`
	ObjectsScanned int                      `json:"objects_scanned"`
	Findings       []processhygiene.Finding `json:"findings"`
	FindingsByRule map[string]int           `json:"findings_by_rule"`
	EmitEvents     bool                     `json:"emit_events_requested"`
}

func runObjectHygieneScan(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		rulesFile, _ := cmd.Flags().GetString("rules-file")
		emitEvents, _ := cmd.Flags().GetBool("emit-events")

		projectRoot, err := requireProjectRoot(proc)
		if err != nil {
			return err
		}
		logger := proc.Logger()

		storageProvider := proc.Storage()
		if storageProvider == nil {
			return errfmt.Errorf("storage not available")
		}
		secCtx := proc.SecurityContext()
		if secCtx == nil {
			secCtx = pkgctx.NewSystemSecurityContext()
		}

		scanCtx, cancel := context.WithTimeout(cmd.Context(), 8*time.Minute)
		defer cancel()

		rules, src, err := processhygiene.ResolveRulesWithOptions(processhygiene.ResolveRulesOptions{
			ProjectRoot:     projectRoot,
			RulesFile:       rulesFile,
			StorageProvider: storageProvider,
			SecCtx:          secCtx,
			ListCtx:         scanCtx,
		})
		if err != nil {
			return errfmt.Newf("load rules").Wrap(err)
		}

		storageCtx := pkgctx.NewStorageContext()

		fr := objects.GetGlobalFieldRegistry()
		if err := fr.LoadFields(); err != nil {
			return errfmt.Newf("load field registry").Wrap(err)
		}
		kinds, err := fr.GetAllKinds()
		if err != nil {
			return errfmt.Newf("list kinds").Wrap(err)
		}

		var all []map[string]any
		scanned := 0
		for _, kind := range kinds {
			qr, err := storageProvider.List(scanCtx, secCtx, storageCtx, storage.ListFilter{
				Kind:  kind,
				Limit: 0,
			})
			if err != nil {
				logging.FluentEvent(logger).Warn("object-hygiene-scan: list kind failed (skipping)").
					Kind(kind).
					WithError(err).
					Log()
				continue
			}
			for _, obj := range qr.Objects {
				scanned++
				all = append(all, obj)
			}
		}

		findings := processhygiene.ScanObjects(all, rules)
		byRule := processhygiene.CountByRule(findings)
		result := objectHygieneScanResult{
			ProjectRoot:    projectRoot,
			RulesSource:    src,
			KindsScanned:   len(kinds),
			ObjectsScanned: scanned,
			Findings:       findings,
			FindingsByRule: byRule,
			EmitEvents:     emitEvents,
		}

		if err := writeObjectHygieneScanOutput(cmd, result); err != nil {
			return err
		}

		logging.FluentEvent(logger).Info("object-hygiene-scan complete").
			Int("objects_scanned", scanned).
			Int("findings", len(findings)).
			String("rules_source", src).
			String("findings_by_rule", fmt.Sprintf("%v", byRule)).
			Log()

		if emitEvents && len(findings) > 0 {
			emitCtx := context.WithoutCancel(cmd.Context())
			goroutinelabels.NewGoroutine("object_hygiene_scan_emit", "emit object-hygiene-scan event").
				StartWithContext(emitCtx, func(ctx context.Context) error {
					emitObjectHygieneScanEvent(ctx, projectRoot, len(findings), byRule)
					return nil
				})
		}

		return nil
	})(cmd, nil)
}

func writeObjectHygieneScanOutput(cmd *cobra.Command, result objectHygieneScanResult) error {
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, result)
	default:
		var buf strings.Builder
		fmt.Fprintf(&buf, "project_root: %s\n", result.ProjectRoot)
		fmt.Fprintf(&buf, "rules_source: %s\n", result.RulesSource)
		fmt.Fprintf(&buf, "kinds_scanned: %d\n", result.KindsScanned)
		fmt.Fprintf(&buf, "objects_scanned: %d\n", result.ObjectsScanned)
		fmt.Fprintf(&buf, "findings: %d\n", len(result.Findings))
		for rule, n := range result.FindingsByRule {
			fmt.Fprintf(&buf, "  %s: %d\n", rule, n)
		}
		for _, f := range result.Findings {
			fmt.Fprintf(&buf, "- %s %s (%s) field=%s %q\n", f.Kind, f.ID, f.RuleID, f.Field, f.Value)
		}
		return cli.WriteOutput(cmd, []byte(buf.String()))
	}
}

func emitObjectHygieneScanEvent(
	ctx context.Context,
	projectRoot string,
	findingsCount int,
	byRule map[string]int,
) {
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	opID := fmt.Sprintf("object_hygiene_scan_%d", time.Now().UnixNano())
	fields := []coordination.LoggingField{
		{Key: "project_root", Value: projectRoot},
		{Key: "findings_count", Value: fmt.Sprintf("%d", findingsCount)},
	}
	for rule, n := range byRule {
		fields = append(fields, coordination.LoggingField{Key: "rule_" + rule, Value: fmt.Sprintf("%d", n)})
	}
	eventData := &coordination.EventData{
		LoggingFields: fields,
		AuditMetadata: map[string]any{
			"findings_count":   findingsCount,
			"findings_by_rule": byRule,
		},
	}
	eventCtx := coordination.NewEventContext(opID, "object_hygiene_scan", "complete").
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, false, false, true)

	globalCoordinator := coordination.GetCoordinator()
	if globalCoordinator == nil {
		log.Debug("object-hygiene-scan: no global coordinator; skip event emit")
		return
	}
	if syncCoordinator, ok := globalCoordinator.(*coordination.Coordinator); ok {
		if err := syncCoordinator.EmitOperationalSync(ctx, eventCtx); err != nil {
			logging.Fluent(log).Debug("object-hygiene-scan: emit event failed").
				String("error", err.Error()).
				Log()
		}
	} else {
		_ = globalCoordinator.Emit(ctx, eventCtx) //nolint:errcheck // best-effort
	}
}
