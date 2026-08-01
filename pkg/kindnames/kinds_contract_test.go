package kindnames_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/kindnames"
	"github.com/lanceman/zqk/pkg/objects"
)

// Lock the contract: pkg/objects Kind* aliases must match canonical kindnames values
// (prevents drift when editing one package but not the other).
func TestObjectsKindAliasesMatchKindnames(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		obj  string
		kn   string
	}{
		{"account", objects.KindAccount, kindnames.Account},
		{"adr", objects.KindAdr, kindnames.Adr},
		{"agent_architecture", objects.KindAgentArchitecture, kindnames.AgentArchitecture},
		{"agent_feed", objects.KindAgentFeed, kindnames.AgentFeed},
		{"agent_onboarding_preparation", objects.KindAgentOnboardingPreparation, kindnames.AgentOnboardingPreparation},
		{"arch_decision_record", objects.KindArchDecisionRecord, kindnames.ArchDecisionRecord},
		{"architectural_decision", objects.KindArchitecturalDecision, kindnames.ArchitecturalDecision},
		{"audit_aggregation_metric", objects.KindAuditAggregationMetric, kindnames.AuditAggregationMetric},
		{"audit_event", objects.KindAuditEvent, kindnames.AuditEvent},
		{"audit_event_aggregation", objects.KindAuditEventAggregation, kindnames.AuditEventAggregation},
		{"auditable", objects.KindAuditable, kindnames.Auditable},
		{"auth_strategy", objects.KindAuthStrategy, kindnames.AuthStrategy},
		{"auto_fix_rule", objects.KindAutoFixRule, kindnames.AutoFixRule},
		{"backlog_item", objects.KindBacklogItem, kindnames.BacklogItem},
		{"base_metric", objects.KindBaseMetric, kindnames.BaseMetric},
		{"base_object", objects.KindBaseObject, kindnames.BaseObject},
		{"base_sampler", objects.KindBaseSampler, kindnames.BaseSampler},
		{"brand", objects.KindBrand, kindnames.Brand},
		{"bucketing_strategy", objects.KindBucketingStrategy, kindnames.BucketingStrategy},
		{"certificate", objects.KindCertificate, kindnames.Certificate},
		{"change_journal_entry", objects.KindChangeJournalEntry, kindnames.ChangeJournalEntry},
		{"code_quality_metric", objects.KindCodeQualityMetric, kindnames.CodeQualityMetric},
		{"code_reference", objects.KindCodeReference, kindnames.CodeReference},
		{"command_metric", objects.KindCommandMetric, kindnames.CommandMetric},
		{"component", objects.KindComponent, kindnames.Component},
		{"context_refresh_schedule", objects.KindContextRefreshSchedule, kindnames.ContextRefreshSchedule},
		{"convergence_session", objects.KindConvergenceSession, kindnames.ConvergenceSession},
		{"corporate_initiative", objects.KindCorporateInitiative, kindnames.CorporateInitiative},
		{"criteria", objects.KindCriteria, kindnames.Criteria},
		{"decision", objects.KindDecision, kindnames.Decision},
		{"department", objects.KindDepartment, kindnames.Department},
		{"display", objects.KindDisplay, kindnames.Display},
		{"division", objects.KindDivision, kindnames.Division},
		{"doc_entry", objects.KindDocEntry, kindnames.DocEntry},
		{"domain_registry", objects.KindDomainRegistry, kindnames.DomainRegistry},
		{"evolution_management", objects.KindEvolutionManagement, kindnames.EvolutionManagement},
		{"extensible_object", objects.KindExtensibleObject, kindnames.ExtensibleObject},
		{"file_lock_metric", objects.KindFileLockMetric, kindnames.FileLockMetric},
		{"goal", objects.KindGoal, kindnames.Goal},
		{"glossary_term", objects.KindGlossaryTerm, kindnames.GlossaryTerm},
		{"glossary_term_relation", objects.KindGlossaryTermRelation, kindnames.GlossaryTermRelation},
		{"impact_analysis", objects.KindImpactAnalysis, kindnames.ImpactAnalysis},
		{"import_tracking", objects.KindImportTracking, kindnames.ImportTracking},
		{"important_date", objects.KindImportantDate, kindnames.ImportantDate},
		{"improvement_report", objects.KindImprovementReport, kindnames.ImprovementReport},
		{"integrity_manifest", objects.KindIntegrityManifest, kindnames.IntegrityManifest},
		{"keystore_entry", objects.KindKeystoreEntry, kindnames.KeystoreEntry},
		{"kind_mapping_metric", objects.KindKindMappingMetric, kindnames.KindMappingMetric},
		{"kind_synonym", objects.KindSynonym, kindnames.KindSynonym},
		{"library", objects.KindLibrary, kindnames.Library},
		{"lifecycle", objects.KindLifecycle, kindnames.Lifecycle},
		{"list_metric_sampler", objects.KindListMetricSampler, kindnames.ListMetricSampler},
		{"mcp_built_in_tool", objects.KindMcpBuiltInTool, kindnames.McpBuiltInTool},
		{"mcp_session", objects.KindMcpSession, kindnames.McpSession},
		{"metadata_package", objects.KindMetadataPackage, kindnames.MetadataPackage},
		{"milestone", objects.KindMilestone, kindnames.Milestone},
		{"mission", objects.KindMission, kindnames.Mission},
		{"namespace", objects.KindNamespace, kindnames.Namespace},
		{"namespace_registry", objects.KindNamespaceRegistry, kindnames.NamespaceRegistry},
		{"object_spec", objects.KindObjectSpec, kindnames.ObjectSpec},
		{"ordered_list_metric_sampler", objects.KindOrderedListMetricSampler, kindnames.OrderedListMetricSampler},
		{"organization", objects.KindOrganization, kindnames.Organization},
		{"organizational_change", objects.KindOrganizationalChange, kindnames.OrganizationalChange},
		{"partnership", objects.KindPartnership, kindnames.Partnership},
		{"persona", objects.KindPersona, kindnames.Persona},
		{"policy", objects.KindPolicy, kindnames.Policy},
		{"priority_plan", objects.KindPriorityPlan, kindnames.PriorityPlan},
		{"prompt_template", objects.KindPromptTemplate, kindnames.PromptTemplate},
		{"question", objects.KindQuestion, kindnames.Question},
		{"release", objects.KindRelease, kindnames.Release},
		{"requirement", objects.KindRequirement, kindnames.Requirement},
		{"resolver", objects.KindResolver, kindnames.Resolver},
		{"roadmap", objects.KindRoadmap, kindnames.Roadmap},
		{"risk_blocker", objects.KindRiskBlocker, kindnames.RiskBlocker},
		{"rollback_report", objects.KindRollbackReport, kindnames.RollbackReport},
		{"role", objects.KindRole, kindnames.Role},
		{"rule", objects.KindRule, kindnames.Rule},
		{"sampler_profile", objects.KindSamplerProfile, kindnames.SamplerProfile},
		{"scalar_metric_sampler", objects.KindScalarMetricSampler, kindnames.ScalarMetricSampler},
		{"scenario", objects.KindScenario, kindnames.Scenario},
		{"scheduler_handler_binding", objects.KindSchedulerHandlerBinding, kindnames.SchedulerHandlerBinding},
		{"scheduler_health_metric", objects.KindSchedulerHealthMetric, kindnames.SchedulerHealthMetric},
		{"scheduler_job", objects.KindSchedulerJob, kindnames.SchedulerJob},
		{"stakeholder_profile", objects.KindStakeholderProfile, kindnames.StakeholderProfile},
		{"status_history_metric_sampler", objects.KindStatusHistoryMetricSampler, kindnames.StatusHistoryMetricSampler},
		{"strategic_context", objects.KindStrategicContext, kindnames.StrategicContext},
		{"strategic_plan", objects.KindStrategicPlan, kindnames.StrategicPlan},
		{"team", objects.KindTeam, kindnames.Team},
		{"technical_debt", objects.KindTechnicalDebt, kindnames.TechnicalDebt},
		{"template", objects.KindTemplate, kindnames.Template},
		{"test_audit_aggregation_metric", objects.KindTestAuditAggregationMetric, kindnames.TestAuditAggregationMetric},
		{"test_case", objects.KindTestCase, kindnames.TestCase},
		{"test_command_rule", objects.KindTestCommandRule, kindnames.TestCommandRule},
		{"verification_matrix", objects.KindVerificationMatrix, kindnames.VerificationMatrix},
		{"vocabulary_scheme", objects.KindVocabularyScheme, kindnames.VocabularyScheme},
		{"vision", objects.KindVision, kindnames.Vision},
		{"workflow", objects.KindWorkflow, kindnames.Workflow},
		{"workstream", objects.KindWorkstream, kindnames.Workstream},
		{"workstream_transition", objects.KindWorkstreamTransition, kindnames.WorkstreamTransition},
		{"zqk_session", objects.KindZqkSession, kindnames.ZqkSession},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.obj != tc.kn {
				t.Fatalf("objects vs kindnames mismatch: %q vs %q", tc.obj, tc.kn)
			}
		})
	}
}
