package testing

import (
	"github.com/zqk-os/zqk/pkg/testservices"
)

// Re-export test service helpers from pkg/testservices for backward compatibility.

// ServiceManager manages test services (e.g., MemGraph)
type ServiceManager = testservices.ServiceManager

// ServiceConfig holds configuration for a test service
type ServiceConfig = testservices.ServiceConfig

// HealthCheck defines how to check if a service is healthy
type HealthCheck = testservices.HealthCheck

// TestServices holds references to running test services
type TestServices = testservices.TestServices

// NewServiceManager creates a new test service manager
var NewServiceManager = testservices.NewServiceManager

// GetMemGraphConfig returns configuration for MemGraph test service
var GetMemGraphConfig = testservices.GetMemGraphConfig

// SetupTestServices sets up all required test services
var SetupTestServices = testservices.SetupTestServices
