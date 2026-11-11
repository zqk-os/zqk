# Bootstrap seed impermeability

Stranger / community `system init` must not seed CAP, MMORCH, orchestrate, or TPM
operator ontology. Studio keeps those surfaces privately; the public bootstrap
archive and community extract path stay pack-free.

## Enforcement

| Layer | Mechanism |
| --- | --- |
| Denylist SSOT | `pkg/bootstrap/community_denylist.go` (`CommunityBootstrapDenylist`) |
| Archive build | `scripts/build-bootstrap-archive.sh` deletes matching staging files before pack |
| Extract filter | Community edition skips denylist hits for all archive entry kinds |
| CI | `scripts/open-core/check-bootstrap-seed-impermeability.sh` greps `pkg/bootstrap/archive/manifest.txt` and runs denylist unit tests |

## Operator note

When adding a new CAP/MMORCH/orchestrate/TPM CLI spec or scheduler template, either
keep it out of the community tree or extend **both** the Go denylist and the
build/CI fragment lists in the same change.
