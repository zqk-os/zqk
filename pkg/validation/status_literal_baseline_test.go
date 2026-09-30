package validation

// statusLiteralBaseline records how many literals currently write each illegal (kind, status)
// pair, so TestStatusLiterals_matchTheLifecycleOfTheKindTheyAreWrittenWith can refuse new ones
// while the existing debt is paid down.
//
// Every entry is a literal asserting a status its kind's lifecycle does not define. Each fails at
// the storage boundary with a lifecycle error naming the kind, which reads like a defect in the
// code under test rather than a wrong fixture — that is how they accumulated, and why several cost
// real debugging time before the vocabulary was checkable at all.
//
// Two production sites this scan found are fixed: pkg/scheduler/convergence_engine.go created
// priority_plan at "planning" (that is evolution_management's origin; priority_plan's is grooming)
// and persisted it only because the call runs under lifecycle break-glass, and
// cmd/zqk/system/evolve.go created evolution_management at "implemented", which that lifecycle does
// not define, so every cycle record was rejected silently because the create error is discarded.
//
// Counts are a ceiling, not a target: the test fails when a count grows or a new pair appears, and
// logs when a count shrinks so the ceiling can be lowered. Do not add pairs — fix the fixture.
//
// backlog_item=invalid_status is the one deliberate entry: three negative tests assert an unknown
// status is rejected, and naming a real status there would defeat the test.
//
// The block below the fixture entries is a different problem and is not fixture debt. Those kinds
// have no lifecycle of their own, so they resolve base_object's — a work ladder
// (proposed/approved/in_progress/implemented/archived/error) with no "active" — while the code
// around them is written as if they were active/inactive registries. Both planes of the schema
// agree there is no "active": the generated status enums under
// pkg/specbuilder/bldr_enum_v1/shared_{watchdog_registrations,scheduler_handler_bindings,remote_kernels}
// list exactly base_object's six. It is the code that assumes a state it was never granted, which is
// why pkg/scheduler/handlers_watchdog_evaluation.go filters watchdog_registration on status=active —
// a query no stored object can match, so that handler is inert.
//
// Two of those entries are production creates, not fixtures: cmd/zqk/system/snapshot_scenario_impl.go
// creates a scenario at "exploring" and pkg/git/integration.go creates a code_reference at "active".
// They are baselined rather than fixed because picking a replacement decides product semantics for
// these kinds (does a harvested code_reference enter at proposed or implemented?), and the same
// choice governs whether base_object should gain "active" or these kinds should get their own
// lifecycles. All of it is unchanged from origin/main, so it is pre-existing debt this scan revealed
// rather than anything the branch introduced.
//
// kinds below it get registry lifecycles, or the code is corrected to the ladder they have.
var statusLiteralBaseline = map[string]int{
	"agent_task=active":           1,
	"audit_event=proposed":        1,
	"backlog_item=active":         12,
	"backlog_item=draft":          5,
	"backlog_item=invalid_status": 3,
	"backlog_item=proposed":       2,
	"criteria=active":             4,
	"criteria=not_started":        6,
	"division=proposed":           2,
	"milestone=active":            5,
	"organization=proposed":       4,
	"persona=active":              1,
	"priority_plan=planned":       1,
	"priority_plan=planning":      1,
	"requirement=draft":           1,
	"requirement=planned":         9,

	// Kinds with no lifecycle of their own, judged against base_object's. See the note above:
	// these are a code-versus-schema mismatch, not wrong fixtures.
	"base_metric=completed": 2,
	// code_reference=active, department=active, partnership=active, remote_kernel=active,
	// scenario=exploring, scheduler_handler_binding=active, shockwave_router=active,
	// team=active, watchdog_registration=active are deliberately absent: their illegal sites
	// are gone and rows removed so reappearance registers as a new pair.
}
