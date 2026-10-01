package system

import (
	"github.com/zqk-os/zqk/pkg/systemcheck/integrity"
)

// objectComplianceSnapshot is the instance-validation health slice of kernel integrity.
// Delegated to pkg/systemcheck/integrity.
type objectComplianceSnapshot = integrity.ObjectComplianceSnapshot
type objectComplianceDelta = integrity.ObjectComplianceDelta
type checkSummaryFile = integrity.CheckSummaryFile
type objectComplianceHistoryEntry = integrity.ObjectComplianceHistoryEntry

var (
	kernelObjectComplianceDir              = integrity.KernelObjectComplianceDir
	kernelObjectComplianceHistoryPath      = integrity.KernelObjectComplianceHistoryPath
	preferredCheckSummaryPaths             = integrity.PreferredCheckSummaryPaths
	loadObjectComplianceSnapshot           = integrity.LoadObjectComplianceSnapshot
	classifyObjectComplianceTrend          = integrity.ClassifyObjectComplianceTrend
	readLastObjectComplianceHistory        = integrity.ReadLastObjectComplianceHistory
	readPenultimateObjectComplianceHistory = integrity.ReadPenultimateObjectComplianceHistory
	readObjectComplianceHistoryTail        = integrity.ReadObjectComplianceHistoryTail
	appendObjectComplianceHistory          = integrity.AppendObjectComplianceHistory
)
