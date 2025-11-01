package models

import (
	"os"
	"strings"
	"testing"
)

func TestPriorityFromIndex(t *testing.T) {
	// Store original order and restore after test
	originalOrder := PriorityOrder
	defer func() {
		PriorityOrder = originalOrder
		buildPriorityRank()
	}()

	tests := []struct {
		name          string
		priorityOrder []string
		index         int
		expected      string
		description   string
	}{
		{
			name:          "valid index 0 default order",
			priorityOrder: []string{"high", "medium", "low"},
			index:         0,
			expected:      "high",
			description:   "highest priority",
		},
		{
			name:          "valid index 1 default order",
			priorityOrder: []string{"high", "medium", "low"},
			index:         1,
			expected:      "medium",
			description:   "medium priority",
		},
		{
			name:          "valid index 2 default order",
			priorityOrder: []string{"high", "medium", "low"},
			index:         2,
			expected:      "low",
			description:   "lowest priority",
		},
		{
			name:          "negative index",
			priorityOrder: []string{"high", "medium", "low"},
			index:         -1,
			expected:      "low",
			description:   "should return lowest priority for negative index",
		},
		{
			name:          "index too high",
			priorityOrder: []string{"high", "medium", "low"},
			index:         5,
			expected:      "low",
			description:   "should return lowest priority for out-of-range index",
		},
		{
			name:          "custom priority order",
			priorityOrder: []string{"critical", "urgent", "normal", "low"},
			index:         1,
			expected:      "urgent",
			description:   "should work with custom priority configuration",
		},
		{
			name:          "single priority",
			priorityOrder: []string{"only"},
			index:         0,
			expected:      "only",
			description:   "should work with single priority level",
		},
		{
			name:          "single priority out of range",
			priorityOrder: []string{"only"},
			index:         1,
			expected:      "only",
			description:   "should return only priority for out-of-range in single-priority system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test priority order
			PriorityOrder = tt.priorityOrder
			buildPriorityRank()

			result := PriorityFromIndex(tt.index)
			if result != tt.expected {
				t.Errorf("PriorityFromIndex(%d) = %q, expected %q (%s)",
					tt.index, result, tt.expected, tt.description)
			}
		})
	}
}

func TestPriorityToIndex(t *testing.T) {
	// Store original order and restore after test
	originalOrder := PriorityOrder
	defer func() {
		PriorityOrder = originalOrder
		buildPriorityRank()
	}()

	tests := []struct {
		name          string
		priorityOrder []string
		priority      string
		expected      int
		description   string
	}{
		{
			name:          "valid priority high",
			priorityOrder: []string{"high", "medium", "low"},
			priority:      "high",
			expected:      0,
			description:   "highest priority should return index 0",
		},
		{
			name:          "valid priority medium",
			priorityOrder: []string{"high", "medium", "low"},
			priority:      "medium",
			expected:      1,
			description:   "medium priority should return index 1",
		},
		{
			name:          "valid priority low",
			priorityOrder: []string{"high", "medium", "low"},
			priority:      "low",
			expected:      2,
			description:   "low priority should return index 2",
		},
		{
			name:          "invalid priority",
			priorityOrder: []string{"high", "medium", "low"},
			priority:      "invalid",
			expected:      3,
			description:   "invalid priority should return len(PriorityOrder)",
		},
		{
			name:          "empty priority",
			priorityOrder: []string{"high", "medium", "low"},
			priority:      "",
			expected:      3,
			description:   "empty priority should return len(PriorityOrder)",
		},
		{
			name:          "custom priority order",
			priorityOrder: []string{"critical", "urgent", "normal", "low"},
			priority:      "urgent",
			expected:      1,
			description:   "should work with custom priority configuration",
		},
		{
			name:          "case sensitive",
			priorityOrder: []string{"high", "medium", "low"},
			priority:      "HIGH",
			expected:      3,
			description:   "should be case sensitive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test priority order
			PriorityOrder = tt.priorityOrder
			buildPriorityRank()

			result := PriorityToIndex(tt.priority)
			if result != tt.expected {
				t.Errorf("PriorityToIndex(%q) = %d, expected %d (%s)",
					tt.priority, result, tt.expected, tt.description)
			}
		})
	}
}

func TestValidatePriorityIndex(t *testing.T) {
	// Store original order and restore after test
	originalOrder := PriorityOrder
	defer func() {
		PriorityOrder = originalOrder
		buildPriorityRank()
	}()

	tests := []struct {
		name          string
		priorityOrder []string
		index         int
		expected      bool
		description   string
	}{
		{
			name:          "valid index 0",
			priorityOrder: []string{"high", "medium", "low"},
			index:         0,
			expected:      true,
			description:   "index 0 should be valid",
		},
		{
			name:          "valid index max",
			priorityOrder: []string{"high", "medium", "low"},
			index:         2,
			expected:      true,
			description:   "max valid index should be valid",
		},
		{
			name:          "invalid negative index",
			priorityOrder: []string{"high", "medium", "low"},
			index:         -1,
			expected:      false,
			description:   "negative index should be invalid",
		},
		{
			name:          "invalid too high index",
			priorityOrder: []string{"high", "medium", "low"},
			index:         3,
			expected:      false,
			description:   "index >= len should be invalid",
		},
		{
			name:          "single priority valid",
			priorityOrder: []string{"only"},
			index:         0,
			expected:      true,
			description:   "should work with single priority",
		},
		{
			name:          "single priority invalid",
			priorityOrder: []string{"only"},
			index:         1,
			expected:      false,
			description:   "should invalidate out-of-range for single priority",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test priority order
			PriorityOrder = tt.priorityOrder
			buildPriorityRank()

			result := ValidatePriorityIndex(tt.index)
			if result != tt.expected {
				t.Errorf("ValidatePriorityIndex(%d) = %t, expected %t (%s)",
					tt.index, result, tt.expected, tt.description)
			}
		})
	}
}

func TestPriorityRoundTrip(t *testing.T) {
	// Store original order and restore after test
	originalOrder := PriorityOrder
	defer func() {
		PriorityOrder = originalOrder
		buildPriorityRank()
	}()

	// Test round-trip conversion for various configurations
	configs := [][]string{
		{"high", "medium", "low"},
		{"critical", "urgent", "normal", "low"},
		{"priority1", "priority2", "priority3", "priority4", "priority5"},
		{"only"},
	}

	for _, config := range configs {
		t.Run("config_"+config[0], func(t *testing.T) {
			PriorityOrder = config
			buildPriorityRank()

			// Test round-trip for all valid indices
			for i := 0; i < len(config); i++ {
				// Index -> Priority -> Index
				priority := PriorityFromIndex(i)
				backToIndex := PriorityToIndex(priority)
				if backToIndex != i {
					t.Errorf("Round-trip failed: %d -> %q -> %d", i, priority, backToIndex)
				}

				// Priority -> Index -> Priority
				originalPriority := config[i]
				index := PriorityToIndex(originalPriority)
				backToPriority := PriorityFromIndex(index)
				if backToPriority != originalPriority {
					t.Errorf("Round-trip failed: %q -> %d -> %q", originalPriority, index, backToPriority)
				}
			}
		})
	}
}

func TestPriorityOrderEnvironmentVariable(t *testing.T) {
	// Store original values
	originalOrder := PriorityOrder
	originalEnv := os.Getenv("PRIORITY_ORDER")
	defer func() {
		PriorityOrder = originalOrder
		buildPriorityRank()
		if originalEnv == "" {
			_ = os.Unsetenv("PRIORITY_ORDER")
		} else {
			_ = os.Setenv("PRIORITY_ORDER", originalEnv)
		}
	}()

	tests := []struct {
		name     string
		envValue string
		expected []string
	}{
		{
			name:     "custom priorities",
			envValue: "critical,urgent,normal,low",
			expected: []string{"critical", "urgent", "normal", "low"},
		},
		{
			name:     "single priority",
			envValue: "only",
			expected: []string{"only"},
		},
		{
			name:     "with spaces",
			envValue: "high, medium, low",
			expected: []string{"high", " medium", " low"}, // Note: spaces preserved
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			_ = os.Setenv("PRIORITY_ORDER", tt.envValue)

			// Re-run init logic
			if env := os.Getenv("PRIORITY_ORDER"); env != "" {
				PriorityOrder = strings.Split(env, ",")
			}
			buildPriorityRank()

			// Check that PriorityOrder was updated
			if len(PriorityOrder) != len(tt.expected) {
				t.Errorf("Expected %d priorities, got %d", len(tt.expected), len(PriorityOrder))
				return
			}

			for i, expected := range tt.expected {
				if PriorityOrder[i] != expected {
					t.Errorf("PriorityOrder[%d] = %q, expected %q", i, PriorityOrder[i], expected)
				}
			}

			// Test that the functions work with the new configuration
			for i := 0; i < len(tt.expected); i++ {
				priority := PriorityFromIndex(i)
				if priority != tt.expected[i] {
					t.Errorf("PriorityFromIndex(%d) = %q, expected %q", i, priority, tt.expected[i])
				}
			}
		})
	}
}
