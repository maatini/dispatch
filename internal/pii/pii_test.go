package pii

import "testing"

func TestMaskEmail(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"user@domain.com", "u***@domain.com"},
		{"a@example.org", "a***@example.org"},
		{"longname@test.de", "l***@test.de"},
		{"invalid", "***"},
		{"@nodomain", "***"},
		{"", "***"},
	}
	for _, tc := range cases {
		got := MaskEmail(tc.input)
		if got != tc.want {
			t.Errorf("MaskEmail(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
