# Bootstrap package

Embedded bootstrap archive for `zqk system init` (BLI-907, REQ-9009, REQ-9010, REQ-9011).

## Portable binary contract (open-core)

- **This source repo is not a user project root.** Do not run `system init` here.
- **Build** embeds `archive/bootstrap.tar.gz` into the community binary (`go:embed`).
- **Init** (user project only) extracts that archive into a *separate* directory the user chooses.
- **Verify at build time:** `make verify-bootstrap` / `TestExtractEmbeddedToTempProject` extracts into a temp dir only.

## Studio rebuild (upstream)

- **Build:** Studio `make bootstrap-archive` runs `scripts/build-bootstrap-archive.sh` (scrubs instance-shaped paths).
- **Source (Studio):** `docs/process/_internal`, `.zqk/cli/specs`, maintenance job templates under `scripts/scheduler_jobs/`.
- **Sync:** copy scrubbed `internal/bootstrap/archive/*` into the public candidate before packaging.
- **Init mapping (community):** extract lands under `docs/architecture/_internal` (+ command specs / scripts / cleanup config) in the *user* project.

After changing specs, rebuild the archive upstream and sync into this tree so `go:embed` picks up the new tarball.
