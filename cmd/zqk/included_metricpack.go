//go:build !zqk_omit_metricpack

package main

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	"github.com/zqk-os/zqk/pkg/metricpack"
	"github.com/zqk-os/zqk/pkg/objects"
)

func init() {
	objects.AddPackOwnedKinds(metricpack.Kinds())
	objects.AddModuleSpecRoot(metricpack.SpecDir)
	objects.AddModuleLifecycleRoot(metricpack.LifecycleDir)
	object.AddPackKindRegistrar(func(objectCmd *cobra.Command) {
		object.RegisterKindCommandsForKinds(objectCmd, metricpack.Kinds())
	})
}
