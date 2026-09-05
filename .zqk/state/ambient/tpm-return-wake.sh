#!/usr/bin/env bash
# One-shot TPM return wake. Invoked by scheduler after the away delay.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"
MSG='ATTN TPM: 3h away window ended. CAP kept the 10m kernel loop; it did not run this Cursor seat. COMMS still fail-closed until AGY seats exist — do not swarm. Read .zqk/state/ambient/away-handoff-20260828.json. Continue R26 lifecycle exam as TPM. Do not mint onto R24. Do not re-enable SCH-cap-night-duty.'
./scripts/wake-cursor-tpm.sh "$MSG" || true
if [[ -x ./bin/zqk-stable ]]; then
  ./bin/zqk-stable feed steer --agent-id cursor-composer --message "$MSG" || true
fi
python3 - <<'PY'
import json, datetime
from pathlib import Path
p = Path("/Users/lanceettl/zqk-restore-clone/.zqk/state/ambient/tpm-return-wake-fired.json")
p.write_text(json.dumps({
  "schema": "zqk_tpm_return_wake_v1",
  "fired_at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
  "handoff": ".zqk/state/ambient/away-handoff-20260828.json",
}, indent=2) + "\n")
print("fired", p)
PY
