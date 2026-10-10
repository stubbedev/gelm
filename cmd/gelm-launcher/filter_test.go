package main

import "testing"

func TestFilterApps(t *testing.T) {
	entries := []appEntry{
		{name: "Files", exec: "nautilus"},
		{name: "Terminal", exec: "foot"},
		{name: "Text Editor", exec: "gedit"},
	}
	cases := []struct {
		name  string
		query string
		want  []appEntry
	}{
		{"empty shows everything", "", entries},
		{"whitespace shows everything", "  ", entries},
		{"substring matches", "text", entries[2:]},
		{"case-insensitive", "TERM", entries[1:2]},
		{"no match empties the grid", "zzz", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterApps(entries, tc.query)
			if len(got) != len(tc.want) {
				t.Fatalf("filterApps(%q) = %v, want %v", tc.query, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("filterApps(%q)[%d] = %v, want %v", tc.query, i, got[i], tc.want[i])
				}
			}
		})
	}
	if filtered := filterApps(entries, "files"); len(filtered) != 1 || filtered[0].exec != "nautilus" {
		t.Error("the filtered entry must keep its exec")
	}
}
