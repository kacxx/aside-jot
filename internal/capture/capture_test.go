package capture

import "testing"

func TestMatch(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{">> some thought", "some thought", true},
		{">> x", "x", true},
		{">> trailing space   \n", "trailing space", true},
		{">> multi\nline", "multi\nline", true},
		{">> >> nested", ">> nested", true},
		{">> ünïcode", "ünïcode", true},
		{">>  two spaces", "two spaces", true},
		{">> \tx", "x", true},
		{">>\tx", "x", true},
		{">>\u00a0\u00a0x", "x", true},
		{"<system-reminder>\nnote\n</system-reminder>\n\n\n>> after reminder", "after reminder", true},
		{"<system-reminder>a</system-reminder>\n<system-reminder>b</system-reminder>\n>>  two", "two", true},

		{">>x", "", false},
		{" >> x", "", false},
		{"\t>> x", "", false},
		{"\\>> x", "", false},
		{"a >> b", "", false},
		{"> quote", "", false},
		{">>", "", false},
		{">> ", "", false},
		{">>  ", "", false},
		{">> \u00a0\n", "", false},
		{"<system-reminder>a</system-reminder>\nhello", "", false},
		{"<system-reminder>unterminated\n>> x", "", false},
		{"<system-reminder>a</system-reminder>\nfix it\n>> not first", "", false},
		{">>\nx", "", false},
		{"", "", false},
		{"fix the bug\n>> not at start", "", false},
		{"》 x", "", false},
	}
	for _, tt := range tests {
		got, ok := Match(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Errorf("Match(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
