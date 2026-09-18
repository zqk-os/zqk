package testing

import "github.com/zqk-os/zqk/pkg/testscan"

// Re-exports from pkg/testscan for backward compatibility (scanner, bundles, scan-tests policy).

type (
	TestFunction       = testscan.TestFunction
	TestBundle         = testscan.TestBundle
	TestWithIndex      = testscan.TestWithIndex
	Scanner            = testscan.Scanner
	TimingDataAccessor = testscan.TimingDataAccessor
	BundleStorage      = testscan.BundleStorage
	BundleFile         = testscan.BundleFile
	SaveOptions        = testscan.SaveOptions
)

const PackageConcurrencyLimitsFileName = testscan.PackageConcurrencyLimitsFileName

var (
	NewScanner                                 = testscan.NewScanner
	NewTimingDataAccessor                      = testscan.NewTimingDataAccessor
	SuggestedTimeoutForPackage                 = testscan.SuggestedTimeoutForPackage
	GetMinTimeoutSecondsForPackage             = testscan.GetMinTimeoutSecondsForPackage
	PackageWantsGoTestParallelOne              = testscan.PackageWantsGoTestParallelOne
	ComputePackageConcurrencyLimitsFromBundles = testscan.ComputePackageConcurrencyLimitsFromBundles
	WritePackageConcurrencyLimitsPatch         = testscan.WritePackageConcurrencyLimitsPatch
	ReadPackageConcurrencyLimitsMap            = testscan.ReadPackageConcurrencyLimitsMap
	MergePackageConcurrencyLimitMaps           = testscan.MergePackageConcurrencyLimitMaps
	NewBundleStorage                           = testscan.NewBundleStorage
)
