package scheduler

// Log events for test_io handler (POL-CODE-007). Prefix matches [JobTypeTestIO].
const (
	LogEventTestIOJobStart                = JobTypeTestIO + "_job_start"
	LogEventTestIOJobCompleted            = JobTypeTestIO + "_job_completed"
	LogEventTestIOTestMessagesParseFailed = JobTypeTestIO + "_test_messages_parse_failed"
	LogEventTestIORouteFailed             = JobTypeTestIO + "_route_failed"
	LogEventTestIOMessageRouted           = JobTypeTestIO + "_message_routed"
	LogEventTestIOLogLevelDefaultDemo     = JobTypeTestIO + "_log_level_default_demo"
	LogEventTestIOLogLevelVerboseDemo     = JobTypeTestIO + "_log_level_verbose_demo"
	LogEventTestIOLogLevelDebugDemo       = JobTypeTestIO + "_log_level_debug_demo"
)
