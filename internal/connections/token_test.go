package connections

import (
	"errors"
	"testing"
)

func TestATokenIsCheckedBeforeItIsKept(t *testing.T) {
	record, err := NewToken("https://GitLab.example.com/acme/", "glpat-x", "oauth2")
	if err != nil || record.URLPrefix != "https://gitlab.example.com/acme" || record.Host != "gitlab.example.com" {
		t.Fatalf("NewToken = %+v, %v", record, err)
	}
	for name, values := range map[string][3]string{
		"no host":       {"acme", "t", ""},
		"empty token":   {"https://github.com/acme", "", ""},
		"two lines":     {"https://github.com/acme", "a\nb", ""},
		"user with ':'": {"https://github.com/acme", "t", "a:b"},
	} {
		if _, err := NewToken(values[0], values[1], values[2]); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
