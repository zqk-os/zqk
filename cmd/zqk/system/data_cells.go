// data_cells.go: zqk system data-cells — list kinds + storage_profile from spec index (data cell identity v1).
// Command structure from spec: .zqk/cli/specs/system/data_cells_command.yaml (builder: bldr_cli_cmd_v1.NewSystemDataCellsCommandBuilder).
package system

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/datacellregistry"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagpkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewDataCellsCmd creates the data-cells command from the command spec.
func NewDataCellsCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemDataCellsCommandBuilder(), &cobra.Command{Use: "data-cells"})
	cli.EnsureCmdAnnotations(cmd)
	cmd.Annotations[cli.AnnotationKeySystemKindValidate] = cli.KindValidateFlagKind
	cmd.Args = cobra.NoArgs
	cmd.RunE = runDataCells
	return cmd
}

// filterDataCellsByKind returns desc unchanged when kind is empty; otherwise one descriptor or an error.
func filterDataCellsByKind(desc []datacell.CellKindDescriptor, kind string) ([]datacell.CellKindDescriptor, error) {
	if kind == "" {
		return desc, nil
	}
	one, ok := datacellregistry.DescriptorForKind(desc, kind)
	if !ok {
		return nil, errfmt.Errorf("no data cell for kind %q (not in spec index)", kind)
	}
	return []datacell.CellKindDescriptor{one}, nil
}

// dataCellJSONRow is the JSON shape for each data cell row (--json / --json-envelope).
type dataCellJSONRow struct {
	Kind                string                               `json:"kind"`
	CellID              string                               `json:"cell_id"`
	StorageProfile      string                               `json:"storage_profile,omitempty"`
	StorageProfileOK    bool                                 `json:"storage_profile_known"`
	WriteMode           string                               `json:"write_mode,omitempty"`
	ReadMode            string                               `json:"read_mode,omitempty"`
	IntegrityMode       string                               `json:"integrity_mode,omitempty"`
	MigrationMode       string                               `json:"migration_mode,omitempty"`
	MembraneTransport   string                               `json:"membrane_transport,omitempty"`
	MembranePolicy      string                               `json:"membrane_policy,omitempty"`
	PrimaryPath         string                               `json:"primary_path"`
	OperationalEnvelope *datacell.OperationalEnvelopeSummary `json:"operational_envelope,omitempty"`
	// OperationalEnvelopeKindAugmented is true when pkg/datacell applies a non-empty per-kind augment for this kind+profile (vs profile defaults only).
	OperationalEnvelopeKindAugmented bool `json:"operational_envelope_kind_augmented"`
	// OperationalEnvelopeKindOverride is true when pkg/datacell applies a non-empty per-kind override (remove/replace profile defaults) for this kind+profile.
	OperationalEnvelopeKindOverride bool `json:"operational_envelope_kind_override"`
}

// testBundleHealthSummary mirrors stream_summary/test_bundle_health.json (scheduler stream summary).
type testBundleHealthSummary struct {
	LineCount        int    `json:"line_count,omitempty"`
	UpdatedAtRFC3339 string `json:"updated_at_rfc3339,omitempty"`
	ByteSize         int64  `json:"byte_size,omitempty"`
	LogicalPath      string `json:"logical_path,omitempty"`
	Kind             string `json:"kind,omitempty"`
	PathAlias        string `json:"path_alias,omitempty"`
}

type dataCellsJSONEnvelope struct {
	Cells                   []dataCellJSONRow        `json:"cells"`
	TestBundleHealthSummary *testBundleHealthSummary `json:"test_bundle_health_summary,omitempty"`
	// StreamStewardshipDrift is non-empty when high_volume_kinds stream entries disagree with spec_index (deploy gate signal).
	StreamStewardshipDrift []string `json:"stream_stewardship_drift,omitempty"`
	// AgentChatChannel is the pilot lite-file + JSONL path summary (not a spec-index cell row).
	AgentChatChannel *agentChatChannelPilotSummary `json:"agent_chat_channel,omitempty"`
	// AgentFeedBindings summarizes process-plane agent_feed objects (CAS), alongside the lite-file pilot.
	AgentFeedBindings *agentFeedBindingsSummary `json:"agent_feed_bindings,omitempty"`
	// StewardCoordinator lists resolved paths for the narrow steward/coordinator slice (enqueue WAL + metrics JSONL).
	StewardCoordinator *stewardCoordinatorSliceSummary `json:"steward_coordinator,omitempty"`
}

// stewardCoordinatorSliceSummary surfaces steward enqueue queue and metrics paths (not spec-index rows).
type stewardCoordinatorSliceSummary struct {
	StewardEnqueueJSONLPath string `json:"steward_enqueue_jsonl_path"`
	StewardMetricsJSONLPath string `json:"steward_metrics_jsonl_path"`
}

// agentChatChannelPilotSummary surfaces resolved paths and lite-file policy for the agent chat channel pilot.
type agentChatChannelPilotSummary struct {
	ConfigPath              string `json:"config_path"`
	EventsJSONLPath         string `json:"events_jsonl_path"`
	SchemaVersion           string `json:"schema_version"`
	Enabled                 bool   `json:"enabled"`
	DeliveryMode            string `json:"delivery_mode,omitempty"`
	FeedID                  string `json:"feed_id,omitempty"`
	MaterializedAt          string `json:"materialized_at,omitempty"`
	ContractSchemaVersion   string `json:"contract_schema_version,omitempty"`
	EventsJSONLPathOverride string `json:"events_jsonl_path_override,omitempty"`
	ConfigFileExists        bool   `json:"config_file_exists"`
}

const agentFeedBindingsMaxList = 20

// agentFeedBindingRow is a minimal projection for operator-facing data-cells output.
type agentFeedBindingRow struct {
	ID           string `json:"id"`
	Title        string `json:"title,omitempty"`
	Status       string `json:"status,omitempty"`
	Enabled      bool   `json:"enabled"`
	DeliveryMode string `json:"delivery_mode,omitempty"`
}

// agentFeedBindingsSummary lists CAS agent_feed bindings (process plane), complementing [agentChatChannelPilotSummary].
type agentFeedBindingsSummary struct {
	TotalCount int                   `json:"total_count"`
	Listed     int                   `json:"listed"`
	Truncated  bool                  `json:"truncated,omitempty"`
	Bindings   []agentFeedBindingRow `json:"bindings,omitempty"`
}

func validateDataCellsFlags(jsonOut, jsonEnvelope, envelopePolicyDryRun bool, kindNonEmpty bool) error {
	if envelopePolicyDryRun {
		if jsonOut || jsonEnvelope || kindNonEmpty {
			return errfmt.Errorf("--envelope-policy-dry-run cannot be combined with --json, --json-envelope, or --kind")
		}
		return nil
	}
	if jsonEnvelope && !jsonOut {
		return errfmt.Errorf("--json-envelope requires --json")
	}
	return nil
}

func buildDataCellJSONRows(projectRoot string, desc []datacell.CellKindDescriptor) []dataCellJSONRow {
	out := make([]dataCellJSONRow, 0, len(desc))
	for _, d := range desc {
		contract, ok := datacell.ContractForProfile(d.ParsedProfile)
		row := dataCellJSONRow{
			Kind:                             d.Kind,
			CellID:                           d.CellID,
			StorageProfile:                   d.StorageProfileWire,
			StorageProfileOK:                 d.HasStorageProfile() && d.ParsedProfile.IsKnown(),
			WriteMode:                        contractField(ok, contract.WriteMode),
			ReadMode:                         contractField(ok, contract.ReadMode),
			IntegrityMode:                    contractField(ok, contract.IntegrityMode),
			MigrationMode:                    contractField(ok, contract.MigrationMode),
			MembraneTransport:                contractField(ok, contract.MembraneTransport),
			MembranePolicy:                   contractField(ok, contract.MembranePolicy),
			PrimaryPath:                      primaryPathForDescriptor(projectRoot, d),
			OperationalEnvelopeKindAugmented: datacell.OperationalEnvelopeHasKindAugment(d.Kind, d.ParsedProfile),
			OperationalEnvelopeKindOverride:  datacell.OperationalEnvelopeHasKindOverride(d.Kind, d.ParsedProfile),
		}
		if env, ok := datacell.OperationalEnvelopeForKind(d.Kind, d.ParsedProfile); ok {
			e := env
			row.OperationalEnvelope = &e
		}
		out = append(out, row)
	}
	return out
}

// operationalEnvelopeTableFooter prints a deduplicated v1 envelope summary for profiles present in desc.
func operationalEnvelopeTableFooter(desc []datacell.CellKindDescriptor) string {
	seen := make(map[datacell.StorageProfile]bool)
	var lines []string
	for _, d := range desc {
		if !d.ParsedProfile.IsKnown() || seen[d.ParsedProfile] {
			continue
		}
		seen[d.ParsedProfile] = true
		env, ok := datacell.OperationalEnvelopeForProfile(d.ParsedProfile)
		if !ok {
			continue
		}
		wire := d.StorageProfileWire
		if wire == "" {
			wire = string(d.ParsedProfile)
		}
		lines = append(lines, fmt.Sprintf("  %s: %s", wire, env.CompactSummary()))
	}
	if len(lines) == 0 {
		return ""
	}
	return "\nOperational envelope (v1 summary by storage_profile):\n" + strings.Join(lines, "\n") + "\n"
}

// loadTestBundleHealthSummary reads stream_summary/test_bundle_health.json when present and non-empty.
func loadTestBundleHealthSummary(projectRoot string) (testBundleHealthSummary, bool) {
	var zero testBundleHealthSummary
	p := filepath.Join(projectRoot, paths.ProjectDataDir, "stream_summary", "test_bundle_health.json")
	b, err := fileutil.ReadFile(p)
	if err != nil || len(b) == 0 {
		return zero, false
	}
	var s testBundleHealthSummary
	if err := json.Unmarshal(b, &s); err != nil {
		return zero, false
	}
	if s.LineCount == 0 && s.UpdatedAtRFC3339 == "" {
		return zero, false
	}
	return s, true
}

// formatTestBundleHealthSummaryFooter formats a footer from a loaded summary. Empty when ok is false.
func formatTestBundleHealthSummaryFooter(s testBundleHealthSummary, ok bool) string {
	if !ok {
		return ""
	}
	return fmt.Sprintf(
		"\nTest-bundle health (rolling health.jsonl): ~%d line(s) indexed in stream summary; updated %s. Details: zqk scheduler test-failures health\n",
		s.LineCount, s.UpdatedAtRFC3339,
	)
}

// testBundleHealthSummaryFooter returns a short operator hint when stream_summary/test_bundle_health.json exists
// (written by scheduler stream summary tooling from health.jsonl). Empty string when missing or invalid.
func testBundleHealthSummaryFooter(projectRoot string) string {
	s, ok := loadTestBundleHealthSummary(projectRoot)
	return formatTestBundleHealthSummaryFooter(s, ok)
}

func runDataCells(cmd *cobra.Command, _ []string) error {
	root := ProjectRootOrResolve("")
	if root == "" {
		return errfmt.Errorf("project root not found; run from repo or pass --project-root")
	}
	jsonOut, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	jsonEnvelope, err := cmd.Flags().GetBool("json-envelope")
	if err != nil {
		return err
	}
	envelopePolicyDryRun, err := cmd.Flags().GetBool("envelope-policy-dry-run")
	if err != nil {
		return err
	}
	kindFilter, err := cmd.Flags().GetString("kind")
	if err != nil {
		return err
	}
	kindFilter = strings.TrimSpace(kindFilter)
	if k, ok := cli.KindCanonicalFromPRERun(cli.KindAnnotKeysSystem, cmd); ok {
		kindFilter = k
	} else if kindFilter != "" {
		var vErr error
		kindFilter, vErr = objects.ResolveAndValidateKindForProject(root, kindFilter)
		if vErr != nil {
			return vErr
		}
	}
	if err := validateDataCellsFlags(jsonOut, jsonEnvelope, envelopePolicyDryRun, kindFilter != ""); err != nil {
		return err
	}
	if envelopePolicyDryRun {
		reps, dryErr := schedulerpkg.DryRunDataCellEnvelopePoliciesForKnownProfiles(root)
		if dryErr != nil {
			return dryErr
		}
		b, marshalErr := json.MarshalIndent(reps, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		return cli.WriteOutput(cmd, append(b, '\n'))
	}
	tSpec := time.Now()
	currentRev := objects.GetGlobalSpecLoader().SpecCacheRevision()
	var desc []datacell.CellKindDescriptor
	if currentRev == 0 {
		var loadErr error
		desc, loadErr = datacellregistry.LoadDataCellDescriptors(root)
		if loadErr != nil {
			return loadErr
		}
	} else {
		rm, loadErr := datacellregistry.DescriptorReadModelForProject(root, currentRev)
		if loadErr != nil {
			return loadErr
		}
		desc = rm.Descriptors()
	}
	specLoadDur := time.Since(tSpec)

	desc, err = filterDataCellsByKind(desc, kindFilter)
	if err != nil {
		return err
	}

	buildStart := time.Now()
	var streamSummaryDur time.Duration

	if jsonOut {
		rows := buildDataCellJSONRows(root, desc)
		var payload any = rows
		if jsonEnvelope {
			env := dataCellsJSONEnvelope{Cells: rows}
			tss := time.Now()
			if s, ok := loadTestBundleHealthSummary(root); ok {
				env.TestBundleHealthSummary = &s
			}
			if drift, err := datacellregistry.HighVolumeStreamStewardshipDrift(root); err != nil {
				return errfmt.Newf("stream stewardship drift").Wrap(err)
			} else if len(drift) > 0 {
				env.StreamStewardshipDrift = drift
			}
			acc, accErr := datacell.ReadAgentChatChannelConfig(root)
			if accErr != nil {
				return errfmt.Newf("agent chat channel config").Wrap(accErr)
			}
			rp := datacell.RuntimeOrganismMembraneReadPaths(root).AllRuntimePaths()
			_, statErr := fileutil.Stat(rp.AgentChatChannelConfig)
			effectiveEvents := datacell.EffectiveAgentChatChannelEventsJSONLPath(root, acc)
			env.AgentChatChannel = &agentChatChannelPilotSummary{
				ConfigPath:              rp.AgentChatChannelConfig,
				EventsJSONLPath:         effectiveEvents,
				SchemaVersion:           acc.SchemaVersion,
				Enabled:                 acc.Enabled,
				DeliveryMode:            acc.DeliveryMode,
				FeedID:                  acc.FeedID,
				MaterializedAt:          acc.MaterializedAt,
				ContractSchemaVersion:   acc.ContractSchemaVersion,
				EventsJSONLPathOverride: acc.EventsJSONLPathOverride,
				ConfigFileExists:        statErr == nil,
			}
			if feedSum := loadAgentFeedBindingsSummary(cli.CommandContextOr(cmd, context.Background()), root); feedSum != nil { // Background: request-or-shutdown derived
				env.AgentFeedBindings = feedSum
			}
			env.StewardCoordinator = &stewardCoordinatorSliceSummary{
				StewardEnqueueJSONLPath: rp.StewardEnqueue,
				StewardMetricsJSONLPath: rp.StewardMetrics,
			}
			streamSummaryDur = time.Since(tss)
			payload = env
		}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		buildDur := time.Since(buildStart) - streamSummaryDur
		emitDataCellsStageMetrics(root, specLoadDur, buildDur, streamSummaryDur, kindFilter)
		return cli.WriteOutput(cmd, append(b, '\n'))
	}
	var b strings.Builder
	if kindFilter != "" {
		fmt.Fprintf(&b, "Data cells (from spec index; filtered to kind %q; v1: cell_id = kind):\n\n", kindFilter)
	} else {
		fmt.Fprintf(&b, "Data cells (from spec index; v1: cell_id = kind):\n\n")
	}
	fmt.Fprintf(&b, "%-40s %-36s %-16s %-34s %s\n", "KIND", "CELL_ID", "STORAGE_PROFILE", "PROFILE_CONTRACT", "PRIMARY_PATH")
	var withProfileContracts int
	var withoutStorageProfile int
	for _, d := range desc {
		sp := d.StorageProfileWire
		if sp == "" {
			sp = "(unset)"
			withoutStorageProfile++
		}
		contractLabel := "(unset)"
		if contract, ok := datacell.ContractForProfile(d.ParsedProfile); ok {
			contractLabel = contract.WriteMode
			withProfileContracts++
		}
		fmt.Fprintf(&b, "%-40s %-36s %-16s %-34s %s\n", d.Kind, d.CellID, sp, contractLabel, primaryPathForDescriptor(root, d))
	}
	fmt.Fprintf(&b, "\nSummary: %d total kinds, %d with profile contracts, %d without storage_profile.",
		len(desc), withProfileContracts, withoutStorageProfile)
	if foot := operationalEnvelopeTableFooter(desc); foot != "" {
		fmt.Fprint(&b, foot)
	}
	if drift, err := datacellregistry.HighVolumeStreamStewardshipDrift(root); err != nil {
		return errfmt.Newf("stream stewardship drift").Wrap(err)
	} else if len(drift) > 0 {
		fmt.Fprintf(&b, "\nStream stewardship (high_volume_kinds stream list vs spec_index):\n")
		for _, line := range drift {
			fmt.Fprintf(&b, "  - %s\n", line)
		}
	}
	tBundle := time.Now()
	bundleSum, bundleOK := loadTestBundleHealthSummary(root)
	streamSummaryDur = time.Since(tBundle)
	if hint := formatTestBundleHealthSummaryFooter(bundleSum, bundleOK); hint != "" {
		fmt.Fprint(&b, hint)
	} else {
		fmt.Fprint(&b, "\n")
	}
	rp := datacell.RuntimeOrganismMembraneReadPaths(root).AllRuntimePaths()
	accText, accErr := datacell.ReadAgentChatChannelConfig(root)
	eventsLine := rp.AgentChatChannelEvents
	if accErr == nil {
		eventsLine = datacell.EffectiveAgentChatChannelEventsJSONLPath(root, accText)
	}
	fmt.Fprintf(&b, "Agent chat channel (pilot): config=%s\n  events (JSONL)=%s\n  (see: zqk system path-cache --show-paths; materialize: zqk system materialize-agent-chat-channel; DATA_CELL_RUNTIME_ORGANISM.md#data-cell-narrative — agent_feed vs CVS measure vs orchestration)\n",
		rp.AgentChatChannelConfig, eventsLine)
	fmt.Fprintf(&b, "Steward coordinator (slice): enqueue JSONL=%s\n  steward metrics (JSONL)=%s\n  (metrics lines when recording enabled; see DATA_CELL_MODEL.md)\n",
		rp.StewardEnqueue, rp.StewardMetrics)
	if feedSum := loadAgentFeedBindingsSummary(cli.CommandContextOr(cmd, context.Background()), root); feedSum != nil { // Background: request-or-shutdown derived
		fmt.Fprint(&b, formatAgentFeedBindingsFooter(feedSum))
	}
	buildDur := time.Since(buildStart) - streamSummaryDur
	emitDataCellsStageMetrics(root, specLoadDur, buildDur, streamSummaryDur, kindFilter)
	return cli.WriteOutput(cmd, []byte(b.String()))
}

func contractField(ok bool, value string) string {
	if !ok {
		return ""
	}
	return value
}

func primaryPathForDescriptor(projectRoot string, d datacell.CellKindDescriptor) string {
	switch d.ParsedProfile {
	case datacell.ProfileStream:
		return datacell.CellStreamOverlayKindDir(projectRoot, d.Kind)
	case datacell.ProfileLightFile:
		// Runtime organism slice: use alias-resolved feature-flags path so primary_path tracks PATH_ALIAS_RESOLUTION
		// (same directory contract as datacell.FeatureFlagsPath; routed through [datacell.RuntimeOrganismMembraneReadPaths]).
		ff := datacell.RuntimeOrganismMembraneReadPaths(projectRoot).FeatureFlagsPath()
		if ff == "" {
			return filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir)
		}
		return filepath.Dir(ff)
	default:
		dir := objects.GetDirectoryFromKind(d.Kind)
		if dir == "" {
			dir = d.Kind + "s"
		}
		return datacell.CellCASPrimaryDir(projectRoot, dir)
	}
}

func loadAgentFeedBindingsSummary(ctx context.Context, projectRoot string) *agentFeedBindingsSummary {
	if ctx == nil {
		ctx = context.Background() // Background: request-or-shutdown derived
	}
	// Temp/test project roots (e.g. t.TempDir under ZQK_TEST_ROOT): use storage without WAL/write-behind
	// so background workers do not race t.TempDir cleanup (see storagpkg.IsTestOrTempProjectRoot).
	var sp storagpkg.ObjectStorageProvider
	var err error
	if storagpkg.IsTestOrTempProjectRoot(projectRoot) {
		sp, err = storagpkg.NewFileObjectStorageForTest(projectRoot)
		if sp != nil {
			defer func() { _ = sp.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
		}
	} else {
		var factory *storagpkg.StorageFactory
		factory, err = storagpkg.NewStorageFactory(ctx, projectRoot)
		if factory != nil {
			sp = factory.GetStorageForKind(objects.KindAgentFeed)
			defer func() { _ = sp.Shutdown(context.Background()) }() // Background: request-or-shutdown derived
		}
	}
	if err != nil {
		return nil
	}
	sec := pkgctx.NewSystemSecurityContext()
	stx := pkgctx.NewStorageContext()
	total, err := sp.Count(ctx, sec, storagpkg.ListFilter{Kind: objects.KindAgentFeed})
	if err != nil {
		return nil
	}
	res, err := sp.List(ctx, sec, stx, storagpkg.ListFilter{
		Kind:    objects.KindAgentFeed,
		Limit:   agentFeedBindingsMaxList,
		SortBy:  objects.FieldKeyUpdatedAt,
		SortAsc: false,
	})
	if err != nil {
		return nil
	}
	rows := make([]agentFeedBindingRow, 0, len(res.Objects))
	for _, o := range res.Objects {
		rows = append(rows, agentFeedBindingRow{
			ID:           stringFromObjectMap(o, objects.FieldKeyID),
			Title:        stringFromObjectMap(o, objects.FieldKeyTitle),
			Status:       stringFromObjectMap(o, objects.FieldKeyStatus),
			Enabled:      boolFromObjectMap(o, objects.FieldKeyEnabled),
			DeliveryMode: stringFromObjectMap(o, objects.FieldKeyDeliveryMode),
		})
	}
	listed := len(rows)
	return &agentFeedBindingsSummary{
		TotalCount: total,
		Listed:     listed,
		Truncated:  total > listed,
		Bindings:   rows,
	}
}

func formatAgentFeedBindingsFooter(sum *agentFeedBindingsSummary) string {
	if sum == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nAgent feed bindings (CAS / agent_feed): %d total", sum.TotalCount)
	if sum.Truncated {
		fmt.Fprintf(&b, " (showing %d most recently updated)", sum.Listed)
	}
	fmt.Fprintf(&b, "; see: zqk object list agent_feed\n")
	if sum.TotalCount == 0 {
		return b.String()
	}
	for _, row := range sum.Bindings {
		title := row.Title
		if title == "" {
			title = "(no title)"
		}
		fmt.Fprintf(&b, "  %s  %s  enabled=%v  delivery_mode=%s  status=%s\n",
			row.ID, title, row.Enabled, row.DeliveryMode, row.Status)
	}
	return b.String()
}

func stringFromObjectMap(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func boolFromObjectMap(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
