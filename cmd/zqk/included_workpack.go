//go:build !zqk_omit_workpack

package main

import "github.com/zqk-os/zqk/pkg/workpack"

func init() {
	workpack.Enable()
}
