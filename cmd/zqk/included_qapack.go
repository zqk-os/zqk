//go:build !zqk_omit_qapack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/qapack"
)

func init() {
	objects.AddPackOwnedKinds(qapack.Kinds())
	objects.AddModuleSpecRoot(qapack.SpecDir)
	objects.AddModuleLifecycleRoot(qapack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, qapack.Kinds())
	})
}
