//go:build !zqk_omit_surfacepack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/surfacepack"
)

func init() {
	objects.AddPackOwnedKinds(surfacepack.Kinds())
	objects.AddModuleSpecRoot(surfacepack.SpecDir)
	objects.AddModuleLifecycleRoot(surfacepack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, surfacepack.Kinds())
	})
}
