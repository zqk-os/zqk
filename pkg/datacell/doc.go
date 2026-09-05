// Package datacell implements one slice of the data cell model: the runtime organism — small,
// project-local JSON/YAML the CLI edits (feature flags, tray shortcuts, built-in hook profile,
// optional runtime manifest). These paths are examples of **lite file** storage (bounded JSON, not
// docs/process CAS objects); see glossary_term GLS-1776253895684744000-7799fa3e. [StorageProfile] constants and [ParseStorageProfile] describe the
// physical profile names used in object_specs (storage_profile) and the spec index. [CellKindDescriptor]
// is the v1 identity view (one cell per kind; CellID == Kind). Load from the spec index via package
// datacellregistry (avoids an import cycle: objects already imports datacell). CLI: zqk system data-cells.
// The full cell abstraction
// (logical unit + operational envelope, multi-kind cells) is documented in
// docs/architecture/DATA_CELL_MODEL.md.
// Paths here are the single source of truth for this slice under .zqk/; use [FeatureFlagsPath],
// [CLIHookProfilePath], [TrayYAMLPath], [RuntimeManifestPath], [AgentChatChannelConfigPath],
// [AgentChatChannelEventsJSONLPath], [StewardEnqueueJSONLPath], [StewardMetricsJSONLPath],
// or [AllRuntimePaths]. Packages outside datacell must resolve stream/CAS layout through
// [CellStreamOverlayKindDir], [CellCASPrimaryDir], or [MembraneReadPaths] methods (CRIT-DATACELL-001);
// [StreamCurrentKindDir] and [CASEntityPrimaryDir] are nucleus helpers used inside this package only.
// See docs/architecture/CRIT_DATACELL_001_STRICT_BOUNDARY.md. See
// DATA_CELL_RUNTIME_ORGANISM.md and BLI-1775890418242630000.
//
// Membrane / coordinator contracts (spec-revision–correlated read models + stewardship enqueue surface):
// see membrane.go, enqueue_maintenance.go, steward_enqueue.go, and steward_enqueue_drain.go.
// [EnqueueStewardMaintenance] and [StewardMaintenanceCoordinator] persist coordinator ops to steward_enqueue.jsonl
// without importing pkg/scheduler; pkg/scheduler.StewardEnqueueCoordinator delegates to the same helper.
// [CellHandle] and [MembraneReadPathsForProfile] group profile + coordinator + membrane paths.
// Operational envelope discovery tokens → scheduler job_type wire values: envelope_token_jobs.go (resolved on envelope tick logs/metrics).
// Stream membrane can pair [StreamMembraneReadPathsWithCoordinator] with scheduler or datacell coordinators.
// Post-retention stream work in pkg/storage enqueues steward JSONL with [MaintenanceOpStreamStewardKind]
// (per kind / phase); see docs/architecture/STREAM_KIND_STEWARDSHIP.md.
//
// DescriptorReadModel (descriptor_read_model.go) is the v1 unified read snapshot for cell
// descriptors from the spec index; datacellregistry loads and caches it (see
// DescriptorReadModelForProject). LoggingCellCoordinator (logging_coordinator.go) logs
// maintenance enqueue for observability; combine with [EnqueueStewardMaintenance] or scheduler coordinators for JSONL persistence.
package datacell
