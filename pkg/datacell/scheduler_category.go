package datacell

// SchedulerCategoryDataCellEnvelope is the reserved scheduler_job category for operational-envelope–scoped
// work (future timer/manual jobs). pkg/scheduler aliases this as CategoryDataCellEnvelope; keep values identical.
const SchedulerCategoryDataCellEnvelope = "data_cell_envelope"

// EnvelopeTickSchedulerJobID is the fixed scheduler_job id for the v1 operational-envelope tick (maintenance;
// scheduler_maintenance_config + scripts/scheduler_jobs/data_cell_envelope_tick_six_hourly.yaml).
const EnvelopeTickSchedulerJobID = "SCH-dce-tick"

// EnvelopeTickJobType is scheduler_job.job_type for the envelope tick. Keep in sync with pkg/scheduler.JobTypeDataCellEnvelopeTick.
const EnvelopeTickJobType = "data_cell_envelope_tick"
