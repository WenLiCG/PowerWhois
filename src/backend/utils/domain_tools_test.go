package utils

import "testing"

func TestRegistrableName(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"mail.company.com", "company.com"},
		{"https://example.com:443/a", "example.com"},
		{"https://sub.sample.co.uk/test", "sample.co.uk"},
		{"www.食狮.中国。", "xn--85x722f.xn--fiqs8s"},
		{"a.example.github.io", "github.io"},
		{"co.uk", ""},
		{"first last.com", ""},
		{"a..com", ""},
		{"-example.com", ""},
		{"127.0.0.1", ""},
	} {
		got, err := TrimAndGetMainDomain(tc.input)
		if got != tc.want || (err != nil) != (tc.want == "") {
			t.Errorf("%q = %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
}
