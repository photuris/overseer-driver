package textutil

import "testing"

func TestShellJoin(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"claude"}, "claude"},
		{[]string{"claude", "--model", "opus"}, "claude --model opus"},
		{[]string{"echo", "hello world"}, "echo 'hello world'"},
		{[]string{"echo", "it's"}, `echo 'it'\''s'`},
	}
	for _, c := range cases {
		if got := ShellJoin(c.in); got != c.want {
			t.Errorf("ShellJoin(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTrimTrailingBlankLines(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"a\nb\n\n\n", "a\nb"},
		{"a\nb", "a\nb"},
		{"\n\n\n", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := TrimTrailingBlankLines(c.in); got != c.want {
			t.Errorf("TrimTrailingBlankLines(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
