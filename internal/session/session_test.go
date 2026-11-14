// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

package session

import "testing"

func TestScrollShift(t *testing.T) {
	sep := "├───┼───┤"
	tests := []struct {
		name      string
		pre, post []string
		want      int
	}{
		{"no change", []string{"a", "b", "c", "d"}, []string{"a", "b", "c", "d"}, 0},
		{"spinner redraw with repeated separator",
			[]string{sep, "x", "y", "z", sep, "⠋ working"},
			[]string{sep, "x", "y", "z", sep, "⠙ working"}, 0},
		{"scroll by one", []string{"a", "b", "c", "d"}, []string{"b", "c", "d", "e"}, 1},
		{"scroll by two with redrawn bottom", []string{"a", "b", "c", "d", "> "}, []string{"c", "d", "e", "f", "> "}, 2},
		{"burst larger than screen", []string{"a", "b", "c"}, []string{"x", "y", "z"}, 3},
		{"blank screen", []string{"", "", ""}, []string{"a", "", ""}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scrollShift(tt.pre, tt.post); got != tt.want {
				t.Errorf("scrollShift = %d, want %d", got, tt.want)
			}
		})
	}
}
