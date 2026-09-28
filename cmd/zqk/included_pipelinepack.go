//go:build !zqk_omit_pipelinepack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/pipelinepack"
)

func init() {
	objects.AddPackOwnedKinds(pipelinepack.Kinds())
	objects.AddModuleSpecRoot(pipelinepack.SpecDir)
	objects.AddModuleLifecycleRoot(pipelinepack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, pipelinepack.Kinds())
	})
}
