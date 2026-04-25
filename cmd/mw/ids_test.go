package main

import "testing"

func TestResolveIDFromIDs(t *testing.T) {
	ids := []string{
		"86ah39gvg", // ends in vg
		"86ah39g4t", // ends in 4t
		"86ah39fkb", // ends in kb
		"86agz2306", // ends in 06
		"86agjtfh0", // ends in h0
		"86abc1206", // ends in 06 — overlaps with 86agz2306 to make "06" ambiguous
	}

	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"86ah39gvg", "86ah39gvg", false}, // full ID, also a self-suffix
		{"gvg", "86ah39gvg", false},       // unique suffix
		{"4t", "86ah39g4t", false},        // unique suffix
		{"fkb", "86ah39fkb", false},       // unique suffix
		{"06", "", true},                  // ambiguous — two IDs end in 06
		{"zzz", "", true},                 // no match, too short
		{"86xxxx999", "86xxxx999", false}, // no match but looks like full ID → pass through
	}

	for _, c := range cases {
		got, err := resolveIDFromIDs(ids, c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("resolveIDFromIDs(%q) = %q; want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("resolveIDFromIDs(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("resolveIDFromIDs(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}
