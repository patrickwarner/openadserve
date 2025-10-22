package main

import (
	"testing"
)

// Basic smoke test to ensure the package compiles
func TestPackageCompiles(t *testing.T) {
	// This test ensures the package compiles without errors
	// More comprehensive tests can be added later with proper mocking
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
}
