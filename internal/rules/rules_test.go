package rules

import "testing"

func TestExactAndSuffix(t *testing.T) {
	m := New()
	m.SetRules([]string{
		"localhost",
		".apple.com",
		"*.local",
		"# comment",
		"",
	})

	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"LOCALHOST", true},
		{"foo.apple.com", true},
		{"apple.com", true},
		{"notapple.com", false},
		{"bar.local", true},
		{"local", true},
		{"example.com", false},
		{"localhost:8080", true},
	}
	for _, c := range cases {
		if got := m.Match(c.host); got != c.want {
			t.Errorf("Match(%q)=%v want %v", c.host, got, c.want)
		}
	}
}

func TestSetFromText(t *testing.T) {
	m := New()
	m.SetFromText("a.com\n.b.com\n")
	if !m.Match("a.com") || !m.Match("x.b.com") {
		t.Fatal("expected matches")
	}
}
