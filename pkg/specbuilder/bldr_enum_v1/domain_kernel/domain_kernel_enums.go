package domain_kernel

import (
	base_objectenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/base_object"
	shared_accountsenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/shared_accounts"
	shared_qasuccessenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/shared_qa_success"
	shared_zqksessionsenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/shared_zqk_sessions"
)

// Consolidated Domain Kernel Enums
// Unifies disparate micro-package enums into a cohesive, typed domain package.

// Plane represents the CAS membrane lifecycle plane.
type Plane = base_objectenum.Plane

const (
	PlaneApoptotic Plane = base_objectenum.PlaneApoptotic
	PlaneDraft     Plane = base_objectenum.PlaneDraft
	PlanePromoted  Plane = base_objectenum.PlanePromoted
	PlaneStaged    Plane = base_objectenum.PlaneStaged
)

// PriorityTier represents ontological priority categorization.
type PriorityTier = base_objectenum.PriorityTier

const (
	PriorityTierP0 PriorityTier = base_objectenum.PriorityTierP0
	PriorityTierP1 PriorityTier = base_objectenum.PriorityTierP1
	PriorityTierP2 PriorityTier = base_objectenum.PriorityTierP2
	PriorityTierP3 PriorityTier = base_objectenum.PriorityTierP3
)

// SourceType represents the origin channel for kernel objects.
type SourceType = base_objectenum.SourceType

const (
	SourceTypeExternal SourceType = base_objectenum.SourceTypeExternal
	SourceTypeImported SourceType = base_objectenum.SourceTypeImported
	SourceTypeInternal SourceType = base_objectenum.SourceTypeInternal
)

// AccountStatus represents lifecycle states for Account objects.
type AccountStatus = shared_accountsenum.Status

const (
	AccountStatusActive     AccountStatus = shared_accountsenum.StatusActive
	AccountStatusConceptual AccountStatus = shared_accountsenum.StatusConceptual
	AccountStatusInactive   AccountStatus = shared_accountsenum.StatusInactive
	AccountStatusOriginated AccountStatus = shared_accountsenum.StatusOriginated
	AccountStatusSuspended  AccountStatus = shared_accountsenum.StatusSuspended
)

// QASuccessStatus represents the verification outcome for QA success tokens.
type QASuccessStatus = shared_qasuccessenum.Status

const (
	QASuccessStatusSuccess QASuccessStatus = shared_qasuccessenum.StatusSuccess
)

// ZqkSessionStatus represents runtime state transitions for agent sessions.
type ZqkSessionStatus = shared_zqksessionsenum.Status

const (
	SessionStatusActive     ZqkSessionStatus = shared_zqksessionsenum.StatusActive
	SessionStatusArchived   ZqkSessionStatus = shared_zqksessionsenum.StatusArchived
	SessionStatusCompleted  ZqkSessionStatus = shared_zqksessionsenum.StatusCompleted
	SessionStatusConceptual ZqkSessionStatus = shared_zqksessionsenum.StatusConceptual
	SessionStatusError      ZqkSessionStatus = shared_zqksessionsenum.StatusError
	SessionStatusOriginated ZqkSessionStatus = shared_zqksessionsenum.StatusOriginated
)
