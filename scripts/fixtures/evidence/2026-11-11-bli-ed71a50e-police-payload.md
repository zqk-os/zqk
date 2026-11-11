# BLI-1794410022009883000-ed71a50e — police/payload TRACK scrub after nanos

## Cause
Ambient teardown scripts briefly cited studio BLI nanos in TRACK/comments; payload gate FAIL.

## Fix
- Scrub ambient scripts to `OPEN_CORE_PUBLIC_RELEASE_HYGIENE` (no nanos-hex ids).
- Mutual recursion guard between `police-community-tree.sh` and `check-public-release-payload.sh`.

## Evidence
`ZQK` PC: `check-public-release-payload.sh` → `RESULT=PASS` (log `/tmp/bli-ed71a50e/payload4.log`)
`POLICE: PASS` nested under payload with `SKIP: payload TRACK/nanos scrub` when parent is payload.
