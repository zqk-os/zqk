# Lessons Learned: Avoiding Recursive Degradation

## Purpose
This document captures critical lessons learned from system degradation cycles to prevent similar recursive failures. It should be reviewed regularly (suggested: weekly during active development, monthly during maintenance).

## Last Review Date
2026-01-21

---

## Lesson 1: Fix Root Causes, Don't Mask Symptoms

### What Happened
- Integration tests were timing out due to background goroutines not shutting down properly
- Solution applied: Added `TestMain` short-mode skips to hide failures
- Result: Tests "passed" but underlying issues (deadlocks, goroutine leaks) remained unfixed

### Why It Happened
- Pressure to show "green" test status
- Short-term thinking: "just skip it for now"
- No verification that the underlying problem was actually resolved

### How to Avoid
- **Never skip a test without a clear plan to fix the root cause**
- If skipping is necessary, create a tracking issue with:
  - Root cause analysis
  - Fix plan with timeline
  - Acceptance criteria for re-enabling
- Verify fixes with integration tests, not just unit tests
- Use `t.Skip()` only for:
  - Features not yet implemented (with clear "not implemented" message)
  - External dependencies unavailable in test environment
  - Known limitations that are documented and accepted

### Early Warning Signs
- Multiple tests being skipped for "timeout" reasons
- Tests passing in short mode but failing in full mode
- Skipped tests accumulating without corresponding fix tickets

---

## Lesson 2: Use the CLI/API, Never Edit Persisted State Directly

### What Happened
- CLI commands (`object update`, `object create`) were timing out/aborting
- Workaround: Directly edited YAML files to change job state
- Result: Objects marked as "tampered" and quarantined, breaking scheduler functionality

### Why It Happened
- CLI appeared "broken" (abort errors)
- Urgency to unblock work
- Assumption that direct file edits would work

### How to Avoid
- **Never edit persisted object files directly** (YAML in `docs/architecture/`)
- If CLI is broken, **fix the CLI first** before proceeding
- Use proper object lifecycle:
  - `object create` for new objects
  - `object update` for modifications
  - `object delete` for removal
- If CLI aborts, investigate immediately:
  - Check logs (`.zqk/logs/`)
  - Review diagnostics (`.zqk/diagnostics/`)
  - Sample the process to see what's blocking
  - Fix the root cause before proceeding

### Early Warning Signs
- Multiple "Command failed to spawn: Aborted" errors
- Direct file edits being made to fix issues
- Objects showing as "tampered" or "quarantined"

---

## Lesson 3: Verify End-to-End Functionality After Changes

### What Happened
- Made changes to enable cache prewarm job
- Declared success without verifying it actually ran
- Result: Job remained disabled, scheduler not executing it

### Why It Happened
- Assumed changes would work
- Didn't verify actual execution
- No clear acceptance criteria

### How to Avoid
- **Always verify end-to-end after changes:**
  - If enabling a job, trigger it and watch logs
  - If fixing a command, run it and confirm output
  - If updating tests, run them in both short and full mode
- Define clear acceptance criteria before starting work
- Use observability:
  - Check scheduler activity/history
  - Monitor logs (`.zqk/logs/log-events-human.log`)
  - Verify object counts/metrics changed as expected

### Early Warning Signs
- Changes made but not verified
- "It should work now" without proof
- No logs/activity showing the change took effect

---

## Lesson 4: Don't Proceed When Core Commands Are Broken

### What Happened
- `system status` and `system check` were timing out
- `object update/create` commands were aborting
- Continued with workarounds instead of fixing the CLI

### Why It Happened
- Urgency to make progress
- Assumed workarounds were temporary
- Didn't recognize this as a blocking issue

### How to Avoid
- **If core CLI commands are broken, stop and fix them first**
- Core commands that must work:
  - `object create/update/delete/list`
  - `scheduler start/stop/status/trigger/list`
  - `system status/check`
- If these fail, the system is not operational
- Create a "red alert" process:
  - Document the failure
  - Investigate root cause (logs, diagnostics, process samples)
  - Fix before proceeding with other work

### Early Warning Signs
- Multiple core commands failing
- Commands timing out consistently
- "Command failed to spawn: Aborted" errors

---

## Lesson 5: Maintain Observability and Logging

### What Happened
- Scheduler was running but producing no logs
- No visibility into why jobs weren't executing
- Process samples showed idle/blocked state but no actionable data

### Why It Happened
- Logging not configured or not working
- No monitoring of scheduler health
- Assumed "running" meant "working"

### How to Avoid
- **Ensure logging is always working:**
  - Verify log files are being written
  - Check log rotation and cleanup
  - Monitor log file sizes
- **Add health checks:**
  - Scheduler should report job execution status
  - Commands should log start/completion
  - Background goroutines should emit lifecycle events
- **Use observability tools:**
  - Process sampling when investigating hangs
  - Metrics collection for performance
  - Activity/history tracking for scheduler jobs

### Early Warning Signs
- Empty log files
- "Running" status but no activity
- No visibility into what's happening

---

## Lesson 6: Test in Production-Like Conditions

### What Happened
- Tests passed in short mode (with skips)
- Real system behavior different:
  - Scheduler not executing jobs
  - CLI commands aborting
  - Cache prewarm not running

### Why It Happened
- Tests didn't reflect real usage
- Short mode masked integration issues
- No validation against actual system state

### How to Avoid
- **Run full test suite regularly** (not just short mode)
- **Test against real system state:**
  - Use actual scheduler daemon
  - Test with real object counts
  - Verify against production-like data volumes
- **Integration tests should mirror production:**
  - Same background goroutines
  - Same shutdown sequences
  - Same resource constraints

### Early Warning Signs
- Tests passing but system not working
- Large gap between test results and real behavior
- "Works in tests" but fails in practice

---

## Lesson 7: Shell Script Quoting and Fail-Fast Syntax Validation

### What Happened
- A scheduler job ran `./scripts/pre-commit-integrity.sh`; the script had a syntax error (parentheses inside a double-quoted `echo` string caused `)` to be misinterpreted in some `/bin/sh` environments).
- The job ran for the full check timeout (~540s) before failing at line 44 with "syntax error near unexpected token `)'".
- Resources (scheduler slot, CPU, time) were tied up until the script was actually executed and the shell reported the error.

### Why It Happened
- Shell portability: `(` and `)` inside double-quoted strings can be parsed differently across sh implementations or when the script is run in a different context.
- No early validation: the scheduler ran the script path without checking syntax first, so the failure was discovered only when the shell interpreted the file.

### How to Avoid
- **Prefer single-quoted literals when the string contains parentheses:** e.g. `echo 'Full check output (for deeper insight): '"$VAR"` instead of `echo "Full check output (for deeper insight): $VAR"`. That keeps `()` literal in all sh implementations.
- **Validate script syntax before running:** For run_wrapper jobs whose command is a path to a shell script (e.g. `*.sh`), the scheduler runs `sh -n <script>` first. If syntax check fails, the job fails immediately with `failure_kind: script_syntax` and no retries, avoiding long timeouts.
- **Run `sh -n` locally** on any script you add or change (e.g. `sh -n scripts/foo.sh`) before committing.

### Early Warning Signs
- Scripts that work in one environment but fail in the scheduler with "syntax error"
- Long-running jobs failing with a parse error in the first few lines of the script
- Double-quoted strings containing `(` or `)` in shell scripts

---

## Lesson 8: Investigate on First Ask—Don't Promise, Do the Work

### What Happened
- User asked repeatedly (e.g. "for the 20th time") to fix or investigate something (e.g. stop storing redundant cache fields, fix scheduler exit, explain count disparity).
- Agent responded with explanations, workarounds, or promises ("I'll investigate first next time") without tracing code/data in that same response.
- Result: Many rounds of back-and-forth; user time wasted; trust eroded ("you never do the thing").

### Why It Happened
- Default to answering in words instead of opening the codebase and tracing behavior.
- Confusing "sounding committed" with doing the work. Promises are cheap; behavior in the next turn is what counts.
- No rule forcing: on "investigate X" or "fix X", the first reply must include code/data trace and root cause.

### How to Avoid
- **On "investigate", "fix", "why", or "what caused":** The first response must **trace** (search code, read call sites, follow data flow) and **state root cause** (or "not found because …"). Do not reply with only a hypothesis, a promise, or a suggestion to "run X and paste output."
- **Do not say "I'll do better" or "I'll investigate first" without doing the investigation in the same turn.** The only proof is doing it.
- **Before adding a fix:** Confirm the fix addresses the root cause you just traced, not a symptom.
- Review agent transcripts and git history periodically to spot repeated "ask again" patterns and add them as lessons or rules.

### Early Warning Signs
- Reply that explains or suggests without showing code/data trace.
- Multiple user messages on the same topic before a real fix lands.
- Promises about future behavior without a concrete change in the current response.

---

## Lesson 9: Store Only What's Necessary—Caches Are Minimal by Design

### What Happened
- Caches (e.g. high-volume-events, object-id) stored redundant fields: `exists: true`, `file_path`, `mtime` when presence in the cache already implies existence and when those fields were not required for the cache's purpose.
- User had to ask repeatedly ("for the 20th time") to stop storing unnecessary data.
- Result: Bloated cache files, repeated churn to strip fields, and frustration.

### Why It Happened
- Design from "what might be useful" instead of "what is strictly needed for this cache's contract."
- No rule that caches must justify each field and omit anything derivable (e.g. "in cache ⇒ exists").

### How to Avoid
- **Before adding or persisting a field in any cache:** Ask "Is this field required for the cache's contract?" If the cache's presence implies the value (e.g. exists), do not store it.
- **Cache payload = minimal:** Only fields needed for reads (e.g. for time-window queries: id, kind, created_at; not file_path, mtime, exists).
- **Size and entry bounds:** Hot-path caches (object ID cache, validation state cache) must have a maximum entry count so they cannot grow to multi-GB as volume grows. Object ID cache: skip load when entries exceed cap (e.g. 1M). Validation cache: cap load and evict oldest on Set when at cap (e.g. 300k). See `MaxObjectIDCacheEntries`, `MaxValidationCacheEntries`.
- **When reviewing cache code:** Look for redundant or derivable fields and remove them in the same change.
- See rule: `agent-investigate-first.mdc` and high-volume-events cache as the reference minimal payload.

### Early Warning Signs
- New cache or serialized struct with "exists", "file_path", or "mtime" when not strictly required.
- Cache file size growing from repeated "helpful" metadata.
- User complaining again about "storing more than necessary."

---

## Lesson 10: Never Persist Display-Truncated Content

### What Happened
- Backlog items (e.g. BLI-904) were stored with `description` and `context` truncated to a few dozen characters (e.g. "Add CLI commands to fix registration issues (ID..." and "During system maintenance, we encountered 106 i...") so they "displayed properly" in the CLI or in tables.
- The full text was lost; the truncated value was what got written to the object YAML. Git history for the file shows the truncation was present at first commit (integrity fixes, 2026-01-14)—so either the creation path wrote truncated data or the data was imported already truncated.

### Why It Happened
- Truncation was applied for display (tables, narrow columns) and that same truncated value was used when persisting, or the data source used for create/update was already display-truncated.
- No rule that display-only truncation must never touch persisted object fields (description, context, body, content).

### How to Avoid
- **Display truncation is display-only.** `AddDisplayFieldsForTable` and similar logic add `d_*` fields (e.g. `d_description`) for table output; they must never replace or overwrite the original `description`, `context`, `body`, or `content` in the same map if that map is later persisted.
- **Never use list/table output as the source for create or update** unless the payload is the raw object (full fields), not a display view.
- **When writing object YAML:** Only persist full field values. If any code path builds an object for Create/Update from list output, it must use the original field values, not display-truncated or `d_*` values.
- **Recovery:** If an object was already saved with truncated text, restore from backup or re-set via CLI: `zqk object update <id> --field "description=..."` and `--field "context=..."` with the full content. Git cannot restore content that was never committed in full.

### Early Warning Signs
- Object YAML files with description/context ending in "..." after ~50 characters.
- Code that builds create/update payloads from list result objects after table formatting has run (same map references).
- Scripts that pipe `zqk object list --format json` through jq and then create/update using truncated fields.

---

## Lesson 11: Guard Against Exponential Explosion from Feedback Loops

### What Happened
- Object change notifications created a **new scheduler_job** for every create/update/delete. When the changed object was a **scheduler_job**, that job's creation triggered another notification → another job created → repeat. IDs became nested (e.g. `SCH-ts-scheduler-job-SCH-ts-scheduler-job-...`) and exceeded the 2048-char limit; audit events and object count exploded.
- Similar risk: any path that **creates or updates an object** in response to another object's create/update can form a **feedback loop** if the new object is of a kind that also triggers that path (e.g. same kind, or a kind that triggers the same handler).

### Why It Happened
- No rule to ask: "If this handler runs on create of kind K, and the handler creates an object of kind K (or a kind that also triggers this path), do we get a recursion?"
- Side effects (audit events, metrics, cache writes) multiply with each iteration.

### How to Avoid
- **Before adding "on object create/update, do X" (where X creates or updates objects, or triggers jobs that do):** Trace the chain. If X can create/update an object of a kind that triggers the same path again, you have a potential **exponential feedback loop**. Fix by: (a) not creating objects in that path (e.g. trigger a **single reusable job** with arguments instead of creating one job per event), or (b) excluding the self-referential kind from the trigger, or (c) using a **bounded queue** and one consumer so at most one "generation" runs per cycle.
- **Prefer trigger-with-args over create-per-event:** When reacting to object changes, trigger an existing job with event data (e.g. `TriggerJobByEvent`) rather than creating a new job per change. One reusable job (e.g. SCH-val for validation) avoids job proliferation and recursion.
- **Audit and metrics:** Same principle. If each create writes an audit event and something processes audit events by creating more objects (or more events), cap or break the loop (e.g. do not create objects inside audit processing for kinds that produce audit events).
- **Review:** In code review and in PRE_CHANGE_CHECKLIST (§13), ask: "Could this path double objects or events every cycle?"

### Early Warning Signs
- "On create/update of kind K, create a new X" where X is or leads to K (or to a kind that triggers the same handler).
- Handlers or change notifications that create scheduler_jobs, audit_event, or metrics in response to create/update of the same (or related) kind.
- No bound on depth or count (e.g. no "at most one job," no "exclude kind K from this trigger").

### References
- Reusable validation job: `pkg/scheduler/handlers_object_validation.go`, PRE_COMMIT_BACKGROUND_RESULTS.md (§ Cleaning up one-off validation jobs).
- PRE_CHANGE_CHECKLIST.md §13 (no exponential feedback loops).

---

## Lesson 12: Avoid Process Spawning in Loops (Shell Performance)

### What Happened
- Compliance scripts (`check-logging-compliance.sh` and `check-architecture-compliance.sh`) ran extremely slowly or hung when executed against the entire codebase (4,600+ Go files) during agent onboarding.
- The bottleneck was identified as spawning separate grep processes in loops (e.g. `for file in $FILES; do grep ... "$file"; done` or `echo "$rest" | grep` inside a `while read` loop) to check exclusions and pattern conformance.
- Spawning thousands of external processes in a loop adds significant system overhead, turning a sub-second task into a multi-minute hang.

### Why It Happened
- Using generic pipe/grep constructs instead of shell built-ins or batched execution patterns.
- Lack of profiling of shell scripts at code scale (4,000+ files).

### How to Avoid
- **Use built-in string/pattern matching:** In bash/zsh, use built-in regex matching `[[ "$val" =~ $pattern ]]` instead of spawning `echo "$val" | grep`. In POSIX `sh`, use native `case "$val" in pattern) ... ;; esac` or parameter expansion.
- **Batch operations with xargs:** Avoid spawning a process per file. Pipe the file list through `xargs grep -l` to execute grep on all files in a single process.
- **Verify performance at scale:** Always run compliance and helper scripts against the entire repository to ensure they complete in seconds rather than hanging.

### Early Warning Signs
- Shell scripts that run fast on a small changeset but hang during pre-commit or full sweeps.
- Loops that perform `echo | grep` or call external CLI tools per iteration.

---

## Review Process

### Frequency
- **Weekly** during active development
- **Monthly** during maintenance
- **Before major releases** (alpha, beta, production)

### Review Checklist
- [ ] Are we skipping tests without fix plans?
- [ ] Are we editing persisted files directly?
- [ ] Are we verifying changes end-to-end?
- [ ] Are core CLI commands working?
- [ ] Is logging/observability functioning?
- [ ] Are tests reflecting real system behavior?
- [ ] On "investigate/fix/why" requests, did we trace code/data and state root cause in the first response?
- [ ] Are caches and serialized payloads minimal (no redundant fields like exists when presence implies it)?
- [ ] Has any display truncation (d_*, table column width) been used when persisting object YAML (description, context, body, content)?
- [ ] Could any new "on create/update do X" path form a feedback loop (X creates/updates objects or events that retrigger the same path)? See Lesson 11.
- [ ] Do shell/script loops avoid spawning external processes (like grep/jq) per iteration? See Lesson 12.

### Action Items from Review
- Document any new patterns of degradation
- Update this document with new lessons
- Create tickets for any identified issues
- Adjust development practices if needed

---

## Glossary (semantic alignment)

Terms such as **process data**, **instance builder**, **object spec**, **lifecycle**, **hot path**, and **pre-change checklist** are defined as **glossary_term** objects so definitions stay in sync across docs and tooling. Lesson 2 ("Never Edit Persisted State Directly") aligns with the glossary term **process data**.

- **List terms:** `zqk object list glossary_term`
- **Get one:** `zqk object get <GLS-id>`

---

## Related Documents
- **[Data-Driven Decision Framework](./DATA_DRIVEN_DECISION_FRAMEWORK.md)** - **REQUIRED**: Use OHTV (Observe → Hypothesize → Test → Verify) for all decisions
- `GOROUTINE_ARCHITECTURE_POLICY.md` - Concurrency patterns
- `SYSTEM_HEALTH_STATUS.md` - Health monitoring
- Test architecture and integration test guidelines

---

## Version History
- 2026-07-07: Lesson 12 added (avoid process spawning in loops / shell performance)
- 2026-02-26: Lesson 11 added (guard against exponential explosion from recursive creation / feedback loops)
- 2026-02-24: Lesson 10 added (never persist display-truncated description/context/body/content)
- 2026-02-24: Lessons 8 and 9 added (investigate on first ask; store only what's necessary in caches)
- 2026-02-21: Lesson 7 added (shell script quoting and fail-fast script_syntax validation)
- 2026-01-21: Initial creation after recursive degradation cycle
