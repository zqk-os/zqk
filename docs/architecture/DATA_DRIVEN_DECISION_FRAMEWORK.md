# Data-Driven Decision Framework

## Purpose
This framework ensures all decisions are based on observed data, measured facts, and verifiable outcomes—not assumptions, guesses, or incomplete information. It prevents regressive decisions by requiring evidence before action.

## Last Review Date
2026-01-21

---

## The Problem

**Current Failure Pattern**: Decisions are made based on:
- Assumptions about system state
- Incomplete information
- "It should work" thinking
- Guesses about root causes
- Proceeding without verification

**Result**: Regressive changes that mask symptoms, violate policies, and degrade system capability.

---

## The Framework: OHTV (Observe → Hypothesize → Test → Verify)

### Phase 1: OBSERVE (Collect Data First)

**Never proceed without data.**

#### 1.1 Check System State
```bash
# Check if system is operational
zqk system status

# Check logs for recent activity
tail -n 100 .zqk/logs/log-events-human.log

# Check process state
ps aux | grep zqk
sample <pid>  # If process appears hung

# Check object counts
zqk system object-count

# Check scheduler status
zqk scheduler status
zqk scheduler list
```

#### 1.2 Collect Relevant Metrics
- What is the current state?
- What changed recently?
- What errors are present?
- What is the actual behavior vs. expected?

#### 1.3 Review Documentation
- What policies apply?
- What patterns are established?
- What lessons learned are relevant?
- What is the approved workflow?

#### 1.4 Document Observations
Record what you observe:
- "Logs show: [actual log entries]"
- "Process sample shows: [call stack]"
- "System status reports: [actual output]"
- "Object count is: [number]"
- "CLI command returns: [actual error/output]"

**Stop if you cannot collect data.** If commands fail, that's data—investigate why they fail before proceeding.

---

### Phase 2: HYPOTHESIZE (Form Data-Driven Hypothesis)

**Hypotheses must be based on observed data, not assumptions.**

#### 2.1 State Hypothesis Clearly
Format: "Based on [observed data], I hypothesize [cause/issue], which would explain [observed behavior]."

**Good Examples:**
- "Based on process sample showing threads in `pthread_cond_wait`, I hypothesize the scheduler is blocked waiting for a mutex, which would explain why jobs aren't executing."
- "Based on logs showing 'Command failed to spawn: Aborted', I hypothesize the CLI is deadlocking during initialization, which would explain why object operations fail."

**Bad Examples:**
- "I think the scheduler needs to be restarted." (No data)
- "The CLI should work now." (Assumption)
- "Let me try editing the YAML directly." (Violates policy, no data)

#### 2.2 Identify Testable Predictions
What would we expect to see if the hypothesis is correct?
- "If the scheduler is blocked, process samples should show threads waiting."
- "If CLI is deadlocking, we should see it hang during `object create`."

#### 2.3 Check Policy Compliance
Before testing, verify:
- Does this approach violate any policies?
- Is there an approved pattern for this?
- What does the documentation say?

---

### Phase 3: TEST (Design Verifiable Test)

**Tests must be small, focused, and verifiable.**

#### 3.1 Design Minimal Test
- Test one thing at a time
- Make it reproducible
- Define success criteria before running

#### 3.2 Execute Test
- Run the test
- Capture all output
- Record actual results (not expected)

#### 3.3 Analyze Results
- Did the test confirm or refute the hypothesis?
- What new data did we collect?
- What does this tell us?

**If test fails or produces unexpected results, return to Phase 1 (Observe) with new data.**

---

### Phase 4: VERIFY (Confirm Outcome)

**Never declare success without verification.**

#### 4.1 Verify the Change Worked
- Did it achieve the stated goal?
- What data confirms this?
- Run the same observations from Phase 1 again

#### 4.2 Check for Side Effects
- Did this break anything else?
- Are there unintended consequences?
- Check logs, metrics, system status

#### 4.3 Document Outcome
- What was the actual result?
- What data supports this?
- What did we learn?

---

## Decision Checklist

Before taking any action, complete this checklist:

### Pre-Action Checklist
- [ ] **OBSERVED**: Collected actual data about current state
  - [ ] System status checked
  - [ ] Logs reviewed
  - [ ] Metrics collected
  - [ ] Process state sampled (if needed)
- [ ] **HYPOTHESIZED**: Formed data-driven hypothesis
  - [ ] Hypothesis based on observed data
  - [ ] Testable predictions identified
  - [ ] Policy compliance verified
- [ ] **TESTED**: Designed and executed verifiable test
  - [ ] Test is minimal and focused
  - [ ] Success criteria defined
  - [ ] Results analyzed
- [ ] **VERIFIED**: Confirmed outcome
  - [ ] Goal achieved (with data)
  - [ ] Side effects checked
  - [ ] Outcome documented

### Red Flags (Stop and Reassess)
- ❌ Proceeding without checking system state
- ❌ Making assumptions about behavior
- ❌ Violating documented policies
- ❌ Skipping verification step
- ❌ "It should work" thinking
- ❌ No data to support decision

---

## Examples

### Example 1: CLI Command Failing

**❌ WRONG APPROACH:**
1. Assume CLI is broken
2. Edit YAML directly as workaround
3. Declare success

**✅ CORRECT APPROACH:**

**OBSERVE:**
```bash
# Collect data
./bin/zqk object create scheduler_job --help  # Does help work?
./bin/zqk object create scheduler_job --data '...'  # What's the actual error?
tail -n 50 .zqk/logs/*.log  # What do logs show?
sample $(pgrep -f zqk)  # Is process hung?
```

**HYPOTHESIZE:**
"Based on 'Command failed to spawn: Aborted', I hypothesize the CLI is deadlocking during command initialization, which would explain why object operations fail."

**TEST:**
```bash
# Test with minimal command
./bin/zqk --version  # Does basic command work?
./bin/zqk object list --limit 1  # Does read work?
strace -e trace=all ./bin/zqk object create ...  # Where does it hang?
```

**VERIFY:**
- If fix applied, run same commands again
- Check logs for successful execution
- Verify object was created via `zqk object list`

---

### Example 2: Scheduler Not Executing Jobs

**❌ WRONG APPROACH:**
1. Assume job needs to be enabled
2. Edit YAML directly
3. Assume it will work

**✅ CORRECT APPROACH:**

**OBSERVE:**
```bash
# Collect data
zqk scheduler status  # What's the actual status?
zqk scheduler list  # What jobs exist? What's their state?
zqk object list scheduler_job --filter id=SCH-007  # What's the job metadata?
tail -n 100 .zqk/logs/log-events-human.log  # What activity is logged?
sample $(pgrep -f scheduler)  # What is scheduler actually doing?
```

**HYPOTHESIZE:**
"Based on logs showing no cache_prewarm activity and job metadata showing `enabled: false`, I hypothesize the job is disabled, which would explain why it's not executing."

**TEST:**
```bash
# Test enabling via CLI (not direct YAML edit)
zqk object update SCH-007 --field enabled=true --dry-run  # Does command work?
# If CLI fails, investigate why before proceeding
```

**VERIFY:**
- Check job metadata: `zqk object list scheduler_job --filter id=SCH-007`
- Check logs for job execution
- Verify cache was warmed (check metrics/object counts)

---

## Integration with Existing Processes

### Architecture Review Process
- Before implementation, complete OHTV for the proposed change
- Document observations and hypotheses in review

### Lessons Learned
- When a decision leads to regression, document:
  - What data was missing?
  - What assumptions were made?
  - How could OHTV have prevented this?

### Policy Compliance
- OHTV ensures policies are checked before action
- Phase 2 (Hypothesize) requires policy review

---

## Review and Improvement

### Weekly Review
- Review decisions made this week
- Identify any that violated OHTV
- Update framework based on learnings

### Monthly Review
- Analyze decision quality trends
- Identify common failure patterns
- Refine framework

### Before Major Changes
- Complete full OHTV cycle
- Document all phases
- Get review if hypothesis is uncertain

---

## Related Documents
- [Lessons Learned](./LESSONS_LEARNED.md) - What happens when we don't follow this
- [Architecture Review Process](./ARCHITECTURE_REVIEW_PROCESS.md) - Integration point
- [AI Agent Onboarding](../../onboarding/AI_AGENT_ONBOARDING.md) - Required reading

---

## Version History
- 2026-01-21: Initial creation after recursive degradation cycle
