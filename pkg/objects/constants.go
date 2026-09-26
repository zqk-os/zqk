package objects

import (
	"regexp"

	"github.com/zqk-os/zqk/pkg/kindnames"
)

// Schema version constants
const (
	// DefaultSchemaVersion is the default schema version for new objects
	// This is used as a fallback when schema version cannot be determined from spec
	DefaultSchemaVersion = "2.0.0"

	// SchemaVersionPattern is the regex pattern for validating schema versions
	// Format: major.minor.patch (e.g., "1.0.0", "2.0.0")
	SchemaVersionPattern = `^\d+\.\d+\.\d+$`

	// InitialFieldVersion is the version assigned when a field is first created (field_versioning).
	InitialFieldVersion = "1.0.0"

	// SessionIDPrefix is the primary zqk_session object id prefix (id_prefixes_config).
	SessionIDPrefix = "ZS-"
)

var schemaVersionRegex = regexp.MustCompile(SchemaVersionPattern)

// ValidSchemaVersion returns version if it matches SchemaVersionPattern, otherwise DefaultSchemaVersion.
// Use when consuming a version from a registry or other source so instance validation never sees invalid schema_version.
func ValidSchemaVersion(version string) string {
	if version != emptyValue && schemaVersionRegex.MatchString(version) {
		return version
	}
	return DefaultSchemaVersion
}

// ObjectStatusArchived is the normalized lifecycle status for archived objects (queries, bulk updates).
const ObjectStatusArchived = "archived"

// ObjectStatusDeleted represents a soft-deleted or terminal deletion status.
const ObjectStatusDeleted = "deleted"

// ObjectStatusNonexistent represents a state where an object does not exist.
const ObjectStatusNonexistent = "nonexistent"

// ObjectStatusActive is the normalized lifecycle status for active objects.
const ObjectStatusActive = "active"

// ObjectStatusConceptual is the normalized lifecycle status for conceptual objects.
const ObjectStatusConceptual = "conceptual"

// ObjectStatusOriginated is the normalized lifecycle status for originated objects.
const ObjectStatusOriginated = "originated"

// ObjectStatusApproved is the normalized lifecycle status for approved objects.
const ObjectStatusApproved = "approved"

// ObjectStatusAnswered is the question lifecycle status after a reply is recorded.
const ObjectStatusAnswered = "answered"

// ObjectStatusOpen is the active tracking status for unresolved risks and blockers.
const ObjectStatusOpen = "open"

// ObjectStatusBlocked is the normalized lifecycle status for blocked objects.
const ObjectStatusBlocked = "blocked"

// ObjectStatusCompleted is the normalized lifecycle status for completed objects.
const ObjectStatusCompleted = "completed"

// ObjectStatusComplete is the lifecycle status for complete objects.
const ObjectStatusComplete = "complete"

// ObjectStatusResolved is the terminal success status for technical_debt (and similar).
const ObjectStatusResolved = "resolved"

// ObjectStatusDeprecated is the normalized lifecycle status for deprecated objects.
const ObjectStatusDeprecated = "deprecated"

// ObjectStatusError is the normalized lifecycle status for error objects.
const ObjectStatusError = "error"

// ObjectStatusFailed is the normalized lifecycle status for failed objects.
const ObjectStatusFailed = "failed"

// ObjectStatusInProgress is the normalized lifecycle status for in-progress objects.
const ObjectStatusInProgress = "in_progress"

// ObjectStatusNotStarted is the normalized lifecycle status for not-started objects.
const ObjectStatusNotStarted = "not_started"

// ObjectStatusPending is the normalized lifecycle status for pending objects.
const ObjectStatusPending = "pending"

// ObjectStatusPendingVerification is the lifecycle status for tasks awaiting verification.
const ObjectStatusPendingVerification = "pending_verification"

// ObjectStatusAwaitingVerification is the ready state for criteria awaiting evidence.
const ObjectStatusAwaitingVerification = "awaiting_verification"

// ObjectStatusPendingImplementation is the lifecycle status for tasks pending implementation.
const ObjectStatusPendingImplementation = "pending_implementation"

// ObjectStatusPlanned is the normalized lifecycle status for planned objects.
const ObjectStatusPlanned = "planned"

// ObjectStatusPlanning is the normalized lifecycle status for planning objects.
const ObjectStatusPlanning = "planning"

// ObjectStatusProcessing is the normalized lifecycle status for processing objects.
const ObjectStatusProcessing = "processing"

// ObjectStatusSuperseded is the normalized lifecycle status for superseded objects.
const ObjectStatusSuperseded = "superseded"

// ObjectStatusAggregated is the change_journal_entry lifecycle status after aggregation (bulk update, list filters).
const ObjectStatusAggregated = "aggregated"

// ObjectStatusApplied is the lifecycle status for applied changes.
const ObjectStatusApplied = "applied"

// ObjectStatusValidated is the normalized lifecycle status for validated objects.
const ObjectStatusValidated = "validated"

// ObjectStatusVerified is the normalized lifecycle status for verified objects.
const ObjectStatusVerified = "verified"

// ObjectStatusUnread is the status for unread items.
const ObjectStatusUnread = "unread"

// ObjectStatusRead is the status for read items.
const ObjectStatusRead = "read"

const ObjectStatusDeferred = "deferred"
const ObjectStatusExploring = "exploring"
const ObjectStatusRoadmap = "roadmap"
const ObjectStatusRejected = "rejected"
const ObjectStatusCancelled = "cancelled"
const ObjectStatusGrooming = "grooming"
const ObjectStatusPrioritizing = "prioritizing"
const ObjectStatusPaused = "paused"
const ObjectStatusDraft = "draft"

// ObjectStatusEscalated is the convergence_session lifecycle status for a session escalated after
// exceeding its liveness window.
const ObjectStatusEscalated = "escalated"

// ObjectStatusIdentified is the initial lifecycle status for technical_debt.
const ObjectStatusIdentified = "identified"
const ObjectStatusInvalidStatus = "invalid_status"

// Object kind constants: aliases of pkg/kindnames (stable objects.Kind* API; add new kinds in kindnames first).
const (
	ConstPersonaOrchestratorAlpha = "PER-ORCH-ALPHA"
	ConstPersonaOrchestratorBeta  = "PER-ORCH-BETA"
	ConstPersonaOrchestratorGamma = "PER-ORCH-GAMMA"
	ConstPersonaDefaultAgent      = "PER-DEFAULT-AGENT"
	ConstPersonaDefaultOperator   = "PER-DEFAULT-OPERATOR"

	DefaultSystemAccountID = "ACC-SYSTEM"

	JobIDCapOrchestrator = "SCH-cap-orchestrator"
	JobIDCapNightDuty    = "SCH-cap-night-duty"

	// StarterCAPGlossaryTermID is the community first-run glossary_term that defines CAP.
	StarterCAPGlossaryTermID = "GLS-STARTER-CAP-001"
	// StarterCAPGlossaryTitle is the portable title used to resolve CAP when CAS ids differ.
	StarterCAPGlossaryTitle = "Continuous Autonomous Progression (CAP)"

	KindBaseObject             = kindnames.BaseObject
	KindAuditable              = kindnames.Auditable
	KindWorkInterval           = kindnames.WorkInterval
	KindWorkUnit               = kindnames.WorkUnit
	KindOccupancy              = kindnames.Occupancy
	KindRemainingOpen          = kindnames.RemainingOpen
	KindBaseMetric             = kindnames.BaseMetric
	KindSchedulerJob           = kindnames.SchedulerJob
	KindCommandMetric          = kindnames.CommandMetric
	KindFileLockMetric         = kindnames.FileLockMetric
	KindAuditAggregationMetric = kindnames.AuditAggregationMetric
	KindChangeJournalEntry     = kindnames.ChangeJournalEntry
	KindAuditEvent             = kindnames.AuditEvent
	KindAuditEventAggregation  = kindnames.AuditEventAggregation
	KindDomainRegistry         = kindnames.DomainRegistry
	KindExtensibleObject       = kindnames.ExtensibleObject
	KindEvolutionManagement    = kindnames.EvolutionManagement
	KindBrand                  = kindnames.Brand
	KindPolicy                 = kindnames.Policy
	KindRemoteKernel           = kindnames.RemoteKernel
	KindAccount                = kindnames.Account
	KindRole                   = kindnames.Role
	KindTeam                   = kindnames.Team
	KindPartnership            = kindnames.Partnership
	KindGoal                   = kindnames.Goal
	KindEpic                   = kindnames.Epic
	KindPriorityPlan           = kindnames.PriorityPlan
	KindQuestion               = kindnames.Question
	KindMilestone              = kindnames.Milestone
	KindCriteria               = kindnames.Criteria
	KindDecision               = kindnames.Decision
	KindRequirement            = kindnames.Requirement
	KindRoadmap                = kindnames.Roadmap
	KindTestCase               = kindnames.TestCase
	KindBacklogItem            = kindnames.BacklogItem
	KindDocEntry               = kindnames.DocEntry
	KindConvergenceSession     = kindnames.ConvergenceSession
	KindPipeline               = kindnames.Pipeline
	KindPipelineDefinition     = kindnames.PipelineDefinition
	KindPipelineExecution      = kindnames.PipelineExecution
	KindProviderProfile        = kindnames.ProviderProfile
	KindTechnicalSpec          = kindnames.TechnicalSpec
	KindCodeFile               = kindnames.CodeFile
	KindCommandSpec            = kindnames.CommandSpec
	KindAgentTask              = kindnames.AgentTask
	KindAgentInstruction       = kindnames.AgentInstruction
	KindLifecycle              = kindnames.Lifecycle
	KindComponent              = kindnames.Component
	KindWorkstream             = kindnames.Workstream
	KindImprovementReport      = kindnames.ImprovementReport
	KindMetricsFeedback        = kindnames.MetricsFeedback
	KindVision                 = kindnames.Vision
	KindMission                = kindnames.Mission
	KindStrategicPlan          = kindnames.StrategicPlan
	KindZqkSession             = kindnames.ZqkSession

	KindObjectSpec        = kindnames.ObjectSpec
	KindTemplate          = kindnames.Template
	KindIntegrityManifest = kindnames.IntegrityManifest
	KindSynonym           = kindnames.KindSynonym

	KindAutoFixRule           = kindnames.AutoFixRule
	KindCodeReference         = kindnames.CodeReference
	KindNamespace             = kindnames.Namespace
	KindScenario              = kindnames.Scenario
	KindSchedulerHealthMetric = kindnames.SchedulerHealthMetric

	KindImportTracking       = kindnames.ImportTracking
	KindOrganization         = kindnames.Organization
	KindDivision             = kindnames.Division
	KindDepartment           = kindnames.Department
	KindOrganizationalChange = kindnames.OrganizationalChange
	KindImpactAnalysis       = kindnames.ImpactAnalysis

	KindTestAuditAggregationMetric = kindnames.TestAuditAggregationMetric
	KindKindMappingMetric          = kindnames.KindMappingMetric
	KindCodeQualityMetric          = kindnames.CodeQualityMetric
	KindNamespaceRegistry          = kindnames.NamespaceRegistry
	KindMetadataPackage            = kindnames.MetadataPackage
	KindCertificate                = kindnames.Certificate
	KindKeystoreEntry              = kindnames.KeystoreEntry
	KindGlossaryTerm               = kindnames.GlossaryTerm
	KindGlossaryTermRelation       = kindnames.GlossaryTermRelation
	KindLibrary                    = kindnames.Library
	KindDisplay                    = kindnames.Display
	KindPersona                    = kindnames.Persona
	KindRelease                    = kindnames.Release
	KindResolver                   = kindnames.Resolver
	KindRollbackReport             = kindnames.RollbackReport
	KindRule                       = kindnames.Rule
	KindSchedulerHandlerBinding    = kindnames.SchedulerHandlerBinding
	KindListMetricSampler          = kindnames.ListMetricSampler
	KindOrderedListMetricSampler   = kindnames.OrderedListMetricSampler
	KindScalarMetricSampler        = kindnames.ScalarMetricSampler
	KindStatusHistoryMetricSampler = kindnames.StatusHistoryMetricSampler
	KindMcpBuiltInTool             = kindnames.McpBuiltInTool
	KindMcpSession                 = kindnames.McpSession
	KindMcpSpec                    = kindnames.McpSpec
	KindAgentArchitecture          = kindnames.AgentArchitecture
	KindAgentFeed                  = kindnames.AgentFeed
	KindAgentOnboardingPreparation = kindnames.AgentOnboardingPreparation
	KindAuthStrategy               = kindnames.AuthStrategy
	KindBaseSampler                = kindnames.BaseSampler
	KindBucketingStrategy          = kindnames.BucketingStrategy
	KindContextRefreshSchedule     = kindnames.ContextRefreshSchedule
	KindSamplerProfile             = kindnames.SamplerProfile
	KindTdeEnvelope                = kindnames.TdeEnvelope
	KindTechnicalDebt              = kindnames.TechnicalDebt
	KindTestCommandRule            = kindnames.TestCommandRule
	KindWorkstreamTransition       = kindnames.WorkstreamTransition
	KindVerificationMatrix         = kindnames.VerificationMatrix

	KindCorporateInitiative   = kindnames.CorporateInitiative
	KindImportantDate         = kindnames.ImportantDate
	KindPromptTemplate        = kindnames.PromptTemplate
	KindStakeholderProfile    = kindnames.StakeholderProfile
	KindStrategicContext      = kindnames.StrategicContext
	KindWorkflow              = kindnames.Workflow
	KindVocabularyScheme      = kindnames.VocabularyScheme
	KindAgentSkill            = kindnames.AgentSkill
	KindCapacityAdvertisement = kindnames.CapacityAdvertisement
	KindComputeAdvertisement  = kindnames.ComputeAdvertisement
	KindEconomicPolicy        = kindnames.EconomicPolicy

	KindRiskBlocker = kindnames.RiskBlocker

	KindInferenceHeuristic = kindnames.InferenceHeuristic
	KindVitalityReport     = kindnames.VitalityReport
	MaturationReport       = kindnames.MaturationReport
	FissionEvent           = kindnames.FissionEvent
	KindReputationScore    = kindnames.ReputationScore
	KindValidationRule     = kindnames.ValidationRule
)

// Missing constants from refactoring
const ObjectStatusRunning = "running"
const ObjectStatusValidationFailed = "validation_failed"
const ObjectStatusUnspecified = "unspecified"

// ObjectStatusImplemented is the normalized lifecycle status for implemented objects.
const ObjectStatusImplemented = "implemented"
const ObjectStatusIssuesFound = "issues_found"
const ObjectStatusProposed = "proposed"
const ObjectStatusCreated = "created"
const ObjectStatusUpdated = "updated"
const ObjectStatusSuccess = "success"
const ObjectStatusTimeout = "timeout"
const ObjectStatusOk = "ok"
const ObjectStatusPass = "pass"
const ObjectStatusNotFound = "not_found"
const ObjectStatusAccepted = "accepted"
const ObjectStatusPromoted = "promoted"
const ObjectStatusAUDITCOMPLETE = "AUDIT_COMPLETE"
const ObjectStatusMERGED = "MERGED"
const ObjectStatusDOCSUPDATED = "DOCS_UPDATED"
const ObjectStatusSUCCESS = "SUCCESS"
const ObjectStatusDOCSGENERATED = "DOCS_GENERATED"
const ObjectStatusHEALTHY = "HEALTHY"
const ObjectStatusReady = "ready"
const ObjectStatusTranslated = "translated"
const ObjectStatusStarted = "started"
const ObjectStatusFlushed = "flushed"
const ObjectStatusStopped = "stopped"
const ObjectStatusExecuting = "executing"
const ObjectStatusSkipped = "skip"
const ObjectStatusDegraded = "degraded"
const ObjectStatusFail = "fail"
const ObjectStatusSkip = "skip"
const ObjectStatusAdded = "added"
const ObjectStatusRemoved = "removed"
const ObjectStatusInstalled = "installed"
const ObjectStatusUninstalled = "uninstalled"
const ObjectStatusCleaned = "cleaned"
