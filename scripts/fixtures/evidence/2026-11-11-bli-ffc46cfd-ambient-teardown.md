# BLI-1794409789128810000-ffc46cfd — ambient temp-epoch teardown

## Gate
`ZQK_BIN=<PC>/bin/zqk sh scripts/open-core/check-community-ambient-teardown-bounded.sh <PC>`

## Result
PASS: ambient teardown — stop clears pid; temp epoch removable
`AMBIENT_TEARDOWN_RESULT ok`

## Smokes updated
- `scripts/open-core/ttfa-unauth-smoke.sh` — `ambient stop` before rm
- `scripts/open-core/test-greenfield-walkthrough.sh` — EXIT trap + `ambient stop`
- `scripts/open-core/check-community-ambient-teardown-bounded.sh` — bounded stranger gate

## Log
`/tmp/bli-ffc46cfd/ambient-teardown.log`
