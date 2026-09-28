//go:build !zqk_omit_agentpack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/agentpack"
	"github.com/zqk-os/zqk/pkg/objects"
)

func init() {
	objects.AddPackOwnedKinds(agentpack.Kinds())
	objects.AddModuleSpecRoot(agentpack.SpecDir)
	objects.AddModuleLifecycleRoot(agentpack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, agentpack.Kinds())
	})
}
