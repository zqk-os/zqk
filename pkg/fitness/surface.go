package fitness

// Surface is a projection context that filters which IssueClass values shout.
type Surface string

const (
	SurfaceAuth          Surface = "auth"
	SurfaceAgentDispatch Surface = "agent_dispatch"
	SurfaceSystemCheckL0 Surface = "system_check_l0"
	SurfaceSystemCheckL1 Surface = "system_check_l1"
	SurfaceAdminForm     Surface = "admin_form"
	SurfaceWhatsNext     Surface = "whats_next"
	SurfaceListDefault   Surface = "list_default"
)

// surfaceAllow is the audience filter for each surface.
// Absent class → suppressed for that surface.
var surfaceAllow = map[Surface]map[IssueClass]struct{}{
	SurfaceAuth: {
		IssueClassEmploymentFitness: {},
		IssueClassProcessFailure:    {},
		IssueClassPolicyGate:        {},
	},
	SurfaceAgentDispatch: {
		IssueClassEmploymentFitness:    {},
		IssueClassProcessFailure:       {},
		IssueClassPolicyGate:           {},
		IssueClassReferentialIntegrity: {},
	},
	SurfaceSystemCheckL0: {
		IssueClassProcessFailure:   {},
		IssueClassDataCompleteness: {},
	},
	SurfaceSystemCheckL1: {
		IssueClassProcessFailure:       {},
		IssueClassDataCompleteness:     {},
		IssueClassReferentialIntegrity: {},
		IssueClassPolicyGate:           {},
	},
	SurfaceAdminForm: {
		IssueClassDataCompleteness:     {},
		IssueClassReferentialIntegrity: {},
	},
	SurfaceWhatsNext: {
		IssueClassProcessFailure:    {},
		IssueClassEmploymentFitness: {},
		IssueClassPolicyGate:        {},
	},
	SurfaceListDefault: {
		// Default list: only process failures — suppress completeness/employment noise.
		IssueClassProcessFailure: {},
	},
}

// VisibleOnSurface reports whether an issue class should surface in the given context.
// Unknown surfaces fail closed (nothing visible) so callers must pass a known Surface.
func VisibleOnSurface(s Surface, c IssueClass) bool {
	allow, ok := surfaceAllow[s]
	if !ok {
		return false
	}
	_, ok = allow[c]
	return ok
}

// FilterClasses returns classes from in that are visible on surface s.
func FilterClasses(s Surface, in []IssueClass) []IssueClass {
	if len(in) == 0 {
		return nil
	}
	out := make([]IssueClass, 0, len(in))
	for _, c := range in {
		if VisibleOnSurface(s, c) {
			out = append(out, c)
		}
	}
	return out
}
