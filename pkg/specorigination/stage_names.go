// Package specorigination defines stable stage names for spec origination pipelines.
//
// Wire these with pkg/pipeline.Builder.AddStage using pipeline kind PipelineKind
// (metrics/logs: pipeline_kind=spec.origination, stage=<name>).
//
// Contract: docs/architecture/DATA_ORIGINATION_PIPELINE_VISION.md (Encapsulation in pkg/pipeline).
// pkg/pipeline.Pipeline.Run executes stages strictly in order; parallel work belongs inside a stage
// or in separate jobs—not as overlapping AddStage order.
package specorigination

// PipelineKind is the recommended pipeline_kind label for metrics and logs.
const PipelineKind = "spec.origination"

// Stage names map origination work onto the lifecycle in docs/architecture/data-pipeline-lifecycle.md.
// Order for AddStage is the contract order below (serial).
const (
	// StageIngest — acquire inputs: draft paths, optional net-new kind file, config patch paths (kind_mappings, id_prefixes, namespaces).
	StageIngest = "SPEC_ORIGIN_INGEST"
	// StageNormalize — pure resolution: LoadSpecWithInheritance, trait merge, storage_profile effective value, no writes.
	StageNormalize = "SPEC_ORIGIN_NORMALIZE"
	// StageDecide — pure plan: which materializations apply (index regen, builders, glossary touch, high_volume_kinds alignment).
	StageDecide = "SPEC_ORIGIN_DECIDE"
	// StageCommit — authoritative writes: write/update object_specs YAML (and internal config patches) via approved tooling only.
	StageCommit = "SPEC_ORIGIN_COMMIT"
	// StageCrossMembrane updates kind mappings, id prefixes, and namespaces based on the new spec.
	StageCrossMembrane = "SPEC_ORIGIN_CROSS_MEMBRANE"
	// StageMaterializeIndexes — write spec_index.json, field-keys outputs, and other derived indexes under process/_internal.
	StageMaterializeIndexes = "SPEC_ORIGIN_MATERIALIZE_INDEXES"
	// StageTriggerSideEffects — idempotent fan-out: generate-instance-builders, sync-glossary-from-specs, path-cache pre-warm, optional detect-spec-changes.
	// TRACK: also append durable contract-change outbox here
	// (lifecycle/object_spec invariant fingerprint); kernel/scheduler start loads + demotes.
	StageTriggerSideEffects = "SPEC_ORIGIN_TRIGGER"
	// StageFinalize — verify: system validate --kind, optional system check --fast, smoke list/count.
	StageFinalize = "SPEC_ORIGIN_FINALIZE"
)
