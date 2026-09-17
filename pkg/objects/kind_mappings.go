package objects

// tdeEnvelopesObjectDirectory is the on-disk folder for KindTdeEnvelope (internal, plural name).
const tdeEnvelopesObjectDirectory = "_internal/tde_envelopes"

// Legacy mappings kept for backward compatibility and as fallback
// These are now used as hints during dynamic discovery
// Note: Some directories have multiple kinds (e.g., "metrics" has base_metric, audit_aggregation_metric, command_metric)
// In those cases, we pick a canonical/default kind for the reverse mapping
var legacyDirectoryToKind = map[string]string{
	"backlog":                   KindBacklogItem,
	"backlog_items":             KindBacklogItem,
	"glossary_terms":            KindGlossaryTerm,
	"glossary_term_relations":   KindGlossaryTermRelation,
	"vocabulary_schemes":        KindVocabularyScheme,
	"goals":                     KindGoal,
	"milestones":                KindMilestone,
	"workstreams":               KindWorkstream,
	"zqk_sessions":              KindZqkSession,
	"priority_plans":            KindPriorityPlan,
	"criteria":                  KindCriteria,
	"requirements":              KindRequirement,
	"tests":                     KindTestCase,
	"test_cases":                KindTestCase,
	"roadmaps":                  KindRoadmap,
	"decisions":                 KindDecision,
	"accounts":                  KindAccount,
	FieldKeyComponents:          KindComponent,
	"convergence_sessions":      KindConvergenceSession,
	"pipelines":                 KindPipeline,
	"pipeline_definitions":      KindPipelineDefinition,
	"pipeline_executions":       KindPipelineExecution,
	"agent_tasks":               KindAgentTask,
	"change_journal":            KindChangeJournalEntry,
	"change_journal_entries":    KindChangeJournalEntry,
	"keystore":                  KindKeystoreEntry,
	"keystore_entries":          KindKeystoreEntry,
	FieldKeyMetrics:             KindCommandMetric, // Default to command_metric for metrics directory (multiple kinds share this)
	"namespace_registries":      KindNamespaceRegistry,
	FieldKeyNamespaces:          KindNamespace,
	"domain_registries":         KindDomainRegistry,
	"domain_registrys":          KindDomainRegistry, // legacy misspelling kept as alias
	"policies":                  KindPolicy,
	"strategic_plans":           KindStrategicPlan,
	"missions":                  KindMission,
	"visions":                   KindVision,
	"strategic_contexts":        KindStrategicContext,
	"stakeholder_profiles":      KindStakeholderProfile,
	FieldKeyImportantDates:      KindImportantDate,
	"audit":                     KindAuditEvent,
	"audit_events":              KindAuditEvent,
	"object_specs":              KindObjectSpec,
	"lifecycles":                KindLifecycle,
	tdeEnvelopesObjectDirectory: KindTdeEnvelope,
	kindSynonymsObjectDirectory: KindSynonym,
}

var legacyKindToDirectory = map[string]string{
	KindBacklogItem:            "backlog_items",
	KindGoal:                   "goals",
	KindMilestone:              "milestones",
	KindWorkstream:             "workstreams",
	KindPriorityPlan:           "priority_plans",
	KindCriteria:               "criteria",
	KindRequirement:            "requirements",
	KindTestCase:               "test_cases",
	KindRoadmap:                "roadmaps",
	KindDecision:               "decisions",
	KindAccount:                "accounts",
	"team_configuration":       "team_configurations",
	MaturationReport:           "maturation_reports",
	KindOrganizationalChange:   "organizational_changes",
	KindImpactAnalysis:         "impact_analyses",
	KindComponent:              "components",
	KindConvergenceSession:     "convergence_sessions",
	KindPipeline:               "pipelines",
	KindPipelineDefinition:     "pipeline_definitions",
	KindPipelineExecution:      "pipeline_executions",
	KindAgentTask:              "agent_tasks",
	KindAgentInstruction:       "agent_instructions",
	KindAuditEvent:             "audit",
	KindChangeJournalEntry:     "change_journal_entries",
	KindBaseMetric:             "metrics",
	KindAuditAggregationMetric: "metrics",
	KindCommandMetric:          "metrics",
	KindCodeReference:          "code_references", // code_reference uses code_references directory
	KindContextRefreshSchedule: "context_refresh_schedules",
	KindCorporateInitiative:    "corporate_initiatives",
	FieldKeyDisplay:            "displays",
	KindDocEntry:               "doc_entries",
	KindGlossaryTerm:           "glossary_terms",
	KindGlossaryTermRelation:   "glossary_term_relations",
	KindVocabularyScheme:       "vocabulary_schemes",
	KindExtensibleObject:       "extensible_objects",
	KindIntegrityManifest:      "integrity_manifests",
	KindMetadataPackage:        "metadata_packages",
	KindPersona:                "personas",
	KindPolicy:                 "policies",
	KindRelease:                "releases",
	KindResolver:               "resolvers",
	KindRiskBlocker:            "risk_blockers",
	FieldKeyRole:               "roles",
	KindRollbackReport:         "rollback_reports",
	KindRule:                   "rules",
	KindScenario:               "scenarios",
	KindSchedulerJob:           "scheduler_jobs",
	KindTemplate:               "templates",
	KindQuestion:               "questions",
	KindMcpSession:             "mcp_sessions",
	KindMcpSpec:                "mcp_specs",
	KindZqkSession:             "zqk_sessions",
	KindKeystoreEntry:          "keystore_entries",
	KindAuthStrategy:           "auth_strategies",
	KindNamespaceRegistry:      "namespace_registries",
	KindNamespace:              "namespaces",
	KindDomainRegistry:         "domain_registries",
	KindStrategicPlan:          "strategic_plans",
	KindMission:                "missions",
	KindVision:                 "visions",
	KindStrategicContext:       "strategic_contexts",
	KindStakeholderProfile:     "stakeholder_profiles",
	KindImportantDate:          FieldKeyImportantDates,
	KindObjectSpec:             "object_specs",
	KindLifecycle:              "lifecycles",
	KindTdeEnvelope:            tdeEnvelopesObjectDirectory,
	KindSynonym:                kindSynonymsObjectDirectory,
}

// GetKindFromDirectory returns the object kind for a given directory name
// Uses dynamic discovery to find the mapping
func GetKindFromDirectory(dirName string) string {
	mapper := GetGlobalKindMapper()
	return mapper.GetKindFromDirectory(dirName)
}

// GetDirectoryFromKind returns the directory name for a given object kind
// Uses dynamic discovery to find the mapping
func GetDirectoryFromKind(kind string) string {
	mapper := GetGlobalKindMapper()
	return mapper.GetDirectoryFromKind(kind)
}
