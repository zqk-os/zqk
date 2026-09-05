package context

// Central labels for ValidationError values returned by chain helpers (DRY; avoids repeated literals).
const (
	validationErrFieldChain            = "Chain"
	validationErrMsgNoContextsInChain  = "no contexts in chain"
	validationErrContextChainBuilder   = "ChainBuilder"
	validationErrFieldContexts         = "Contexts"
	validationErrMsgNoContextsProvided = "no contexts provided"
	validationErrContextBuildChain     = "BuildContextChain"
	validationErrFieldParent           = "Parent"
	validationErrMsgParentRequired     = "parent context is required"
	validationErrContextBuildNested    = "BuildNestedContextChain"
)
