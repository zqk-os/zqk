package crud

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

// QueryFactory provides pre-configured, standard query templates for common storage query patterns.
type QueryFactory struct{}

// DefaultQueryFactory is a global QueryFactory instance.
var DefaultQueryFactory = QueryFactory{}

// Query returns the standard QueryFactory.
func Query() QueryFactory {
	return DefaultQueryFactory
}

// NewQueryFactory creates a new QueryFactory.
func NewQueryFactory() QueryFactory {
	return QueryFactory{}
}

// Builder returns a new blank QueryBuilder for the specified kind.
func (qf QueryFactory) Builder(kind string) *QueryBuilder {
	return NewQueryBuilder(kind)
}

// IdTitle returns a builder configured to project only id and title.
func (qf QueryFactory) IdTitle(kind string) *QueryBuilder {
	return NewQueryBuilder(kind).IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle)
}

// IdTitleStatus returns a builder configured to project id, title, and status.
func (qf QueryFactory) IdTitleStatus(kind string) *QueryBuilder {
	return NewQueryBuilder(kind).IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus)
}

// IdTitleStatusActive returns a builder projecting id, title, and status, with status=active filter.
func (qf QueryFactory) IdTitleStatusActive(kind string) *QueryBuilder {
	return NewQueryBuilder(kind).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus).
		Status(objects.ObjectStatusActive)
}

// Active returns a builder filtered to status=active.
func (qf QueryFactory) Active(kind string) *QueryBuilder {
	return NewQueryBuilder(kind).Status(objects.ObjectStatusActive)
}

// NotArchived returns a builder filtered to exclude archived objects.
func (qf QueryFactory) NotArchived(kind string) *QueryBuilder {
	return NewQueryBuilder(kind).StatusNot(objects.ObjectStatusArchived)
}

// ActiveNotComplete returns a builder filtered to exclude complete, archived, and cancelled objects.
func (qf QueryFactory) ActiveNotComplete(kind string) *QueryBuilder {
	return NewQueryBuilder(kind).StatusNotIn(objects.ObjectStatusComplete, objects.ObjectStatusArchived, "cancelled")
}

// ById returns a builder matching a specific object id with Limit=1.
func (qf QueryFactory) ById(kind string, id string) *QueryBuilder {
	return NewQueryBuilder(kind).Id(id).Limit(1)
}

// ByStatus returns a builder matching a specific status.
func (qf QueryFactory) ByStatus(kind string, status string) *QueryBuilder {
	return NewQueryBuilder(kind).Status(status)
}

// IdTitleStatusNotArchived returns a builder configured with id, title, status projection and status != archived.
func (qf QueryFactory) IdTitleStatusNotArchived(kind string) *QueryBuilder {
	return NewQueryBuilder(kind).
		IncludeFields(objects.FieldKeyID, objects.FieldKeyTitle, objects.FieldKeyStatus).
		StatusNot(objects.ObjectStatusArchived)
}

// ForPlan returns a builder matching items linked to the specified priority plan.
func (qf QueryFactory) ForPlan(kind string, planID string) *QueryBuilder {
	return NewQueryBuilder(kind).AndFilter(objects.FieldKeyPriorityPlanRef, planID)
}

// ForPlanNotArchived returns a builder matching items for a plan where status != archived.
func (qf QueryFactory) ForPlanNotArchived(kind string, planID string) *QueryBuilder {
	return NewQueryBuilder(kind).
		AndFilter(objects.FieldKeyPriorityPlanRef, planID).
		StatusNot(objects.ObjectStatusArchived)
}

// ForPlanNonTerminal returns a builder matching items for a plan excluding terminal statuses.
func (qf QueryFactory) ForPlanNonTerminal(kind string, planID string) *QueryBuilder {
	return NewQueryBuilder(kind).
		AndFilter(objects.FieldKeyPriorityPlanRef, planID).
		StatusNotIn(
			objects.ObjectStatusComplete,
			objects.ObjectStatusCompleted,
			objects.ObjectStatusArchived,
			"cancelled",
			"rejected",
		)
}

// ForMilestone returns a builder matching items linked to the specified milestone.
func (qf QueryFactory) ForMilestone(kind string, milestoneID string) *QueryBuilder {
	return NewQueryBuilder(kind).AndFilter(objects.FieldKeyMilestoneRefs, milestoneID)
}

// ForRequirement returns a builder matching items linked to the specified requirement.
func (qf QueryFactory) ForRequirement(kind string, reqID string) *QueryBuilder {
	return NewQueryBuilder(kind).AndFilter(objects.FieldKeyRequirementRefs, reqID)
}
