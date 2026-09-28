//go:build !zqk_omit_releasepack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/releasepack"
)

func init() {
	objects.AddPackOwnedKinds(releasepack.Kinds())
	objects.AddModuleSpecRoot(releasepack.SpecDir)
	objects.AddModuleLifecycleRoot(releasepack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, releasepack.Kinds())
	})
}
