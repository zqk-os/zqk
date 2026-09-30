package domain_platform

import (
	shared_auditenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/shared_audit"
	shared_policiesenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/shared_policies"
	shared_rulesenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/shared_rules"
	shared_schedulerjobsenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/shared_scheduler_jobs"
)

// Consolidated Domain Platform Enums
// Unifies platform daemon, scheduling, policy, and audit micro-packages
// in accordance with TDE-CEF-F-ARCH-005.

// AuditStatus represents lifecycle states for audit events.
type AuditStatus = shared_auditenum.Status

const (
	AuditStatusArchived   AuditStatus = shared_auditenum.StatusArchived
	AuditStatusCompleted  AuditStatus = shared_auditenum.StatusCompleted
	AuditStatusConceptual AuditStatus = shared_auditenum.StatusConceptual
	AuditStatusError      AuditStatus = shared_auditenum.StatusError
	AuditStatusFailed     AuditStatus = shared_auditenum.StatusFailed
	AuditStatusOriginated AuditStatus = shared_auditenum.StatusOriginated
	AuditStatusPending    AuditStatus = shared_auditenum.StatusPending
	AuditStatusReverted   AuditStatus = shared_auditenum.StatusReverted
)

// PolicyStatus represents lifecycle states for system and security policies.
type PolicyStatus = shared_policiesenum.Status

const (
	PolicyStatusActive     PolicyStatus = shared_policiesenum.StatusActive
	PolicyStatusArchived   PolicyStatus = shared_policiesenum.StatusArchived
	PolicyStatusDraft      PolicyStatus = shared_policiesenum.StatusDraft
	PolicyStatusOriginated PolicyStatus = shared_policiesenum.StatusOriginated
)

// RuleStatus represents lifecycle states for validation and auto-fix rules.
type RuleStatus = shared_rulesenum.Status

const (
	RuleStatusApproved    RuleStatus = shared_rulesenum.StatusApproved
	RuleStatusArchived    RuleStatus = shared_rulesenum.StatusArchived
	RuleStatusConceptual  RuleStatus = shared_rulesenum.StatusConceptual
	RuleStatusError       RuleStatus = shared_rulesenum.StatusError
	RuleStatusImplemented RuleStatus = shared_rulesenum.StatusImplemented
	RuleStatusInProgress  RuleStatus = shared_rulesenum.StatusInProgress
	RuleStatusOriginated  RuleStatus = shared_rulesenum.StatusOriginated
	RuleStatusProposed    RuleStatus = shared_rulesenum.StatusProposed
)

// SchedulerJobStatus represents execution states for background scheduler jobs.
type SchedulerJobStatus = shared_schedulerjobsenum.Status

const (
	SchedulerJobStatusActive     SchedulerJobStatus = shared_schedulerjobsenum.StatusActive
	SchedulerJobStatusArchived   SchedulerJobStatus = shared_schedulerjobsenum.StatusArchived
	SchedulerJobStatusConceptual SchedulerJobStatus = shared_schedulerjobsenum.StatusConceptual
	SchedulerJobStatusDisabled   SchedulerJobStatus = shared_schedulerjobsenum.StatusDisabled
	SchedulerJobStatusError      SchedulerJobStatus = shared_schedulerjobsenum.StatusError
	SchedulerJobStatusOriginated SchedulerJobStatus = shared_schedulerjobsenum.StatusOriginated
	SchedulerJobStatusPending    SchedulerJobStatus = shared_schedulerjobsenum.StatusPending
)
