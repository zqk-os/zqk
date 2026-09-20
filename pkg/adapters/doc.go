// Package adapters is the vendor-neutral factory for host/IDE message delivery.
//
// Kernel code depends on [MessageDelivery] and [Resolve] only. Concrete path
// layouts, clipboard tools, and AppleScript live in pkg/adapters/<vendor>
// (see docs/architecture/CLI_COMMAND_TAXONOMY_STANDARDS.md Rule 3).
package adapters
