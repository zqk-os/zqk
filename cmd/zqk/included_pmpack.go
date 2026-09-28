//go:build !zqk_omit_pmpack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pmpack"
)

func init() {
	objects.AddPackOwnedKinds(pmpack.Kinds())
	objects.AddModuleSpecRoot(pmpack.SpecDir)
	objects.AddModuleLifecycleRoot(pmpack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, pmpack.Kinds())
	})
}
