// Package zqkenv exposes brand-prefixed environment variable names for the zqk executable.
// Prefer zqkenv.Foo() at os.Getenv / Setenv / Unsetenv / t.Setenv call sites instead of "ZQK_…" literals.
package zqkenv
