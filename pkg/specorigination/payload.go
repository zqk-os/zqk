package specorigination

import (
	"github.com/lanceman/zqk/pkg/objects"
)

// State carries state between stages and is returned from [Run] on success.
type State struct {
	Opts     Options
	Loader   *objects.SpecLoader
	Spec     *objects.Spec
	SpecPath string
}
