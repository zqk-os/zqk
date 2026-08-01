package system

// Flag names for [NewEmitContextEventCmd] / emit_context_event_command.yaml.
// The builder is generated from the spec; these must stay in lockstep.
const (
	emitContextEventFlagEventType    = "event-type"
	emitContextEventFlagSource       = "source"
	emitContextEventFlagCorrelation  = "correlation-id"
	emitContextEventFlagCvs          = "cvs-id"
	emitContextEventFlagJob          = "job-id"
	emitContextEventFlagCriteriaRefs = "criteria-refs"
	emitContextEventFlagBliRefs      = "bli-refs"
	emitContextEventFlagNote         = "note"
	emitContextEventFlagPayloadJSON  = "payload-json"
)
