package paths

// PathResolver resolves path references for a single project root using the path alias cache
// (when built). It is the injectable façade over [ResolvePathStrict] and
// [ResolvePathFromCacheOrConstant] so callers can depend on an interface in tests.
//
// Call [storage.BuildPathAliasCacheForProject] (or scheduler pre-warm / zqk system path-cache)
// before [PathResolver.ResolveStrict] when the cache must hit real aliases.
//
// See docs/architecture/PATH_ALIAS_RESOLUTION.md.
type PathResolver interface {
	ProjectRoot() string
	ResolveStrict(pathRef string) (string, error)
	ResolveFromCacheOrConstant(alias, fallbackRel string) string
}

type pathResolver struct {
	root string
}

// NewPathResolver returns a [PathResolver] for projectRoot. An empty root is allowed;
// [ResolveStrict] follows [ResolvePathStrict] behavior for empty roots.
func NewPathResolver(projectRoot string) PathResolver {
	return &pathResolver{root: projectRoot}
}

func (r *pathResolver) ProjectRoot() string {
	if r == nil {
		return ""
	}
	return r.root
}

func (r *pathResolver) ResolveStrict(pathRef string) (string, error) {
	if r == nil {
		return "", ErrPathAliasNotInCache
	}
	return ResolvePathStrict(r.root, pathRef)
}

func (r *pathResolver) ResolveFromCacheOrConstant(alias, fallbackRel string) string {
	if r == nil {
		return ""
	}
	return ResolvePathFromCacheOrConstant(r.root, alias, fallbackRel)
}
