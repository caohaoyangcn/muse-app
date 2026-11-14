package jav

import "testing"

// The extension hands over 番号 exactly as JavDB rendered them. LookupKey must
// only erase presentation differences (case, spacing) and Muse's own FC2
// spelling, and must leave everything else untouched — it is not a parser.
func TestLookupKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ABP-576", "ABP-576"},
		{"  abp-576\t", "ABP-576"},
		{"FC2-PPV-1234567", "FC2-1234567"},
		{"fc2-ppv-004970405", "FC2-004970405"},
		{"FC2-1234567", "FC2-1234567"},
		{"FC2-PPV-", "FC2-PPV-"},
		{"SSIS-001-2", "SSIS-001-2"},
		{"ABP\u200b-576", "ABP-576"},
		{"\u3000ABP-576\u00a0", "ABP-576"},
		{"ABP\u00a0576", "ABP 576"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := LookupKey(c.in); got != c.want {
			t.Errorf("LookupKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
