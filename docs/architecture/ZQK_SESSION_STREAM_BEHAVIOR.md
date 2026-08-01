# zqk_session Stream Behavior

## Why the stream may show no new entries after a date

The **zqk_session** kind is stream-backed (see `pkg/storage/stream_config.go`). Stream segment files under `.zqk/streams/zqk_session/` (e.g. `2026-03-03_stream.json`) only grow when a **new** session is **created** (i.e. when `StartZqkSession` is called and appends to the stream via `Create`).

- **New session** → `StartZqkSession` → `sp.Create(...)` → `writeObjectToStream` → `AppendToStream` → new line in the date’s segment file.
- **Reused session** → `TryReuseSession` returns an existing ID → no `StartZqkSession`, no `Create`, no stream append. Only `TouchSession` (Update) runs; stream-backed Updates do not append new lines to the segment file (they go through WAL/write-behind and do not add new stream records).

So if `.zqk/state/session` (or `ZQK_SESSION_ID`) has contained the same session ID since a given date, every subsequent CLI run reuses that session and only touches it. No new stream writes occur until that session is cleared (or expired by idle timeout) and a new session is created.

**Summary:** Lack of new zqk_session stream writes after a date is expected when the same session is reused. To see new stream entries, clear the persisted session (e.g. remove `.zqk/state/session` or unset `ZQK_SESSION_ID`) or wait for idle timeout to expire so the next run creates a new session.

## Runtime updates: stream_current + change journal (no CAS)

For stream-backed kinds (including zqk_session), **updates** (e.g. TouchSession) are **not** written to CAS. That avoids hash compute and content-addressed file overhead for routine state changes. Instead:

- **Change journal** — each update creates a `change_journal_entry` (object_ref=`zqk_session:<id>`, change_type=update, previous_state, diff_summary) for audit and rollback.
- **Stream-current overlay** — the current state is written to `.zqk/state/stream_current/zqk_session/<id>.yaml` (overwrite, no hash). `Read` uses this file when present; otherwise it uses the initial stream record.

So you will not see new hash-named files under `docs/architecture/zqk_sessions/` for routine touches; those were a side effect of the previous write-behind→CAS path and are no longer used for stream-backed updates. See `docs/architecture/HIGH_VOLUME_STORAGE_DEPRECATION.md` and `pkg/storage/stream_current_state.go`.
