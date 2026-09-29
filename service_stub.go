//go:build !windows

package main

// isService always returns false on non-Windows platforms
func isService() bool { return false }

// runService is a no-op on non-Windows platforms
func runService() {}
