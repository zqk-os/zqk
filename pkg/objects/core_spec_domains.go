package objects

// CoreSpecDomains are the object-spec buckets that stay in the kernel.
// Additive kinds are linked packs: work, pm, qa, agent, and org.
// Account and role stay in the kernel bucket. Authentication resolves both
// even when an additive pack is omitted from the composition root.
var CoreSpecDomains = []string{"kernel", "platform"}
