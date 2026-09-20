package specorigination

// Options configures [BuildSpecOriginationPipeline] and [Run].
type Options struct {
	// ProjectRoot is the zqk project root (directory containing go.mod or .zqk).
	ProjectRoot string

	// Ontology is the object kind stem (e.g. test_case); object_specs/<ontology>.yaml must exist.
	Ontology string

	// DryRun skips MATERIALIZE writes (spec index, field_keys.go) when true.
	DryRun bool

	// SkipMaterializeSpecIndex skips [objects.RefreshMaterializedSpecIndex] even when DryRun is false.
	SkipMaterializeSpecIndex bool

	// MaterializeFieldKeys writes pkg/objects/field_keys.go when true (expensive; touches VCS).
	MaterializeFieldKeys bool

	// SkipFinalizeValidation skips SPEC_ORIGIN_FINALIZE strict [objects.ValidateLoadedSpec] failure.
	// When true, validation errors are recorded in pipeline outcome but do not fail the run.
	SkipFinalizeValidation bool

	// ApplyTrigger runs SPEC_ORIGIN_TRIGGER subprocess steps (glossary apply, instance builders, path-cache)
	// when false [stageTrigger] only records manual command hints. Ignored when DryRun is true.
	ApplyTrigger bool
}
