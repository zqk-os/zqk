# CEF package skeleton — overwrite contract

**cef_version:** 0.1.0

Every file here is a **stub**. Agents **must overwrite** placeholders before claiming a stage done.

| Marker | Meaning |
|--------|---------|
| `REPLACE_ME` | Required string/id still unset |
| `UNGRADED` | Axis grade not yet assigned (Integrator only) |
| `TODO_OVERWRITE` | Narrative / log section still empty |
| Finding id `F-STUB-000` | Delete before handoff; never accept |

**Do not** invent GPA axes. Use Diamond axes only: RDB MNT TST REL OBS RCV SEC ROB.

Materialize with:
`sh ./scripts/cef/materialize-package.sh <output_home> <AGENT_ID> <freeze_sha>`
