package objects

// CoreSpecDomains are the object-spec buckets that stay in the kernel.
// Additive packs linked from the composition root are named for the domain a
// developer would look up: work, org, vocabulary, library, decision, release,
// qa, agent, metric, pipeline, display, interface, workflow, and evolution.
// Account and role stay in the kernel bucket. Authentication resolves both
// even when an additive pack is omitted from the composition root.
// object_spec, lifecycle, extensible_object, and base_metric stay in the
// platform bucket. They are the spec and metric bases other packs extend.
var CoreSpecDomains = []string{"kernel", "platform"}
