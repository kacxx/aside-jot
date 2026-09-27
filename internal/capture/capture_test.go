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

		{">>x", "", false},
		{" >> x", "", false},
		{"\t>> x", "", false},
		{"\\>> x", "", false},
		{"a >> b", "", false},
		{"> quote", "", false},
		{">>", "", false},
		{">> ", "", false},
		{">>  two spaces", "", false},
		{">> \tx", "", false},
		{">>\tx", "", false},
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
