package wlsession

import "testing"

func TestParseFileCursor(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want fileCursor
		ok   bool
	}{
		{fileCursorPrefix + "/c.cur#3,4", fileCursor{path: "/c.cur", hx: 3, hy: 4}, true},
		{fileCursorPrefix + "/c.cur#0,0,pointer", fileCursor{path: "/c.cur", fallback: "pointer"}, true},
		{fileCursorPrefix + "/c.cur#2147483647,1", fileCursor{path: "/c.cur", hx: 2147483647, hy: 1}, true},
		{fileCursorPrefix + "/c.cur#2147483648,1", fileCursor{}, false},
		{fileCursorPrefix + "/c.cur#4294967299,1", fileCursor{}, false},
		{fileCursorPrefix + "/c.cur#-1,1", fileCursor{}, false},
		{fileCursorPrefix + "/c.cur#x,1", fileCursor{}, false},
		{fileCursorPrefix + "/c.cur#1", fileCursor{}, false},
		{fileCursorPrefix + "#1,1", fileCursor{}, false},
		{"pointer", fileCursor{}, false},
	} {
		got, ok := parseFileCursor(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseFileCursor(%q) = %+v, %v; want %+v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
