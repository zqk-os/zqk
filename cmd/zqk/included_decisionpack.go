//go:build !zqk_omit_decisionpack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/decisionpack"
	"github.com/zqk-os/zqk/pkg/objects"
)

func init() {
	objects.AddPackOwnedKinds(decisionpack.Kinds())
	objects.AddModuleSpecRoot(decisionpack.SpecDir)
	objects.AddModuleLifecycleRoot(decisionpack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, decisionpack.Kinds())
	})
}
