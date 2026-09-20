package objects

var kindsRequiringDescription = map[string]struct{}{
	KindBacklogItem:   {},
	KindRequirement:   {},
	KindPriorityPlan:  {},
	KindWorkstream:    {},
	KindPersona:       {},
	KindGoal:          {},
	KindTechnicalDebt: {},
	KindAgentTask:     {},
	KindCriteria:      {},
	KindDecision:      {},
	KindScenario:      {},
	KindRoadmap:       {},
	KindMilestone:     {},
}

// KindRequiresDescription returns true if the kind mandates a substantive description
// to cross the CAS membrane into storage.
func KindRequiresDescription(kind string) bool {
	_, ok := kindsRequiringDescription[kind]
	return ok
}
