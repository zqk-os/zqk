//go:build !zqk_omit_workflowpack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/workflowpack"
)

func init() {
	objects.AddPackOwnedKinds(workflowpack.Kinds())
	objects.AddModuleSpecRoot(workflowpack.SpecDir)
	objects.AddModuleLifecycleRoot(workflowpack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, workflowpack.Kinds())
	})
}
