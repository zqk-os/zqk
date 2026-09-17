# Vendor & Proxy Outage Resilience

ZQK currently relies on `go mod download` with Go modules proxy caches (e.g., `proxy.golang.org`) instead of checking in a `vendor/` directory.

## Outage Mitigation Strategy

1. **Proxy Caching:** By default, Go proxies cache downloaded modules. If upstream repositories (e.g., GitHub) become temporarily unavailable, the Go proxy will continue to serve the cached module versions.
2. **Local Caching:** CI runners utilizing `actions/setup-go` cache downloaded modules across workflow runs, shielding against short-term proxy outages.
3. **Hermetic Builds:** Go 1.26 toolchains and dependencies are strictly pinned (`go.mod`), ensuring that builds remain reproducible and do not drift during recovery.
4. **Future `vendor/` Adoption:** If supply chain instability increases, we will transition to `go mod vendor` to embed dependencies directly in the repository. Currently, the repository size and diff bloat concerns outweigh the benefits, but the architecture is vendor-ready.

## Recovery Procedure
If `proxy.golang.org` experiences a catastrophic outage:
- Switch `GOPROXY` to an alternative public proxy (e.g., `GOPROXY=https://goproxy.cn,direct` or `https://gocenter.io`).
- Or rely on GitHub Actions caches until the primary proxy recovers.
