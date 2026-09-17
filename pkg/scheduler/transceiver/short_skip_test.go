package transceiver

// This file previously contained a package-level TestMain that skipped all tests
// in short mode. That has been removed - individual integration tests now use
// testing.Short() checks where appropriate. Unit tests run in short mode.
