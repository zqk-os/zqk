package scheduler

// decodeScheduledJobPayload decodes a pipeline stage payload that is a pointer-to-struct with a
// *ScheduledJob Job field. P must be the pointer type (e.g. *runWrapperPayload). Wrong type, nil
// payload, or nil Job yields false.
func decodeScheduledJobPayload[P any](payload any, getJob func(P) *ScheduledJob) (P, bool) {
	p, ok := payload.(P)
	if !ok {
		var z P
		return z, false
	}
	if getJob(p) == nil {
		var z P
		return z, false
	}
	return p, true
}
