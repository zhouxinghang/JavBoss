package server

import "testing"

func TestParseDirectoryScanForce(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"no", false},
		{"  ", false},
		{"1", true},
		{"true", true},
		{"True", true},
		{"  TRUE  ", true},
		{"yes", true},
		{"force", true},
	} {
		if got := parseDirectoryScanForce(tc.value); got != tc.want {
			t.Errorf("parseDirectoryScanForce(%q) = %t, want %t", tc.value, got, tc.want)
		}
	}
}
