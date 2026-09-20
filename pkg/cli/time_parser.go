package cli

import (
	"github.com/zqk-os/zqk/pkg/filter"
)

// ResolveTimeTokens is a re-export of filter.ResolveTimeTokens for backward compatibility.
func ResolveTimeTokens(val any) any {
	return filter.ResolveTimeTokens(val)
}
