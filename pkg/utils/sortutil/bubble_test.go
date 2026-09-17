package sortutil

import (
	"testing"
)

func TestBubbleSort(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{
			name:     "unsorted array",
			input:    []int{5, 2, 9, 1, 5, 6},
			expected: []int{1, 2, 5, 5, 6, 9},
		},
		{
			name:     "already sorted",
			input:    []int{1, 2, 3, 4, 5},
			expected: []int{1, 2, 3, 4, 5},
		},
		{
			name:     "empty array",
			input:    []int{},
			expected: []int{},
		},
		{
			name:     "single element",
			input:    []int{42},
			expected: []int{42},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputCopy := make([]int, len(tt.input))
			copy(inputCopy, tt.input)

			BubbleSort(inputCopy, func(a, b int) bool {
				return a < b
			})

			for i, v := range inputCopy {
				if v != tt.expected[i] {
					t.Errorf("expected %v at index %d, got %v", tt.expected[i], i, v)
				}
			}
		})
	}
}
