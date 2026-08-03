package store

import (
	"regexp"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// pattern pulls the compiled regex out of the filter like() builds.
func pattern(t *testing.T, value string) primitive.Regex {
	t.Helper()
	e := like("name", value)
	r, ok := e.Value.(primitive.Regex)
	if !ok {
		t.Fatalf("like() produced %T, want primitive.Regex", e.Value)
	}
	return r
}

// Search terms come from an unauthenticated query string. If they reach Mongo
// as a live pattern, "(a+)+$" backtracks for an unbounded time and ".*" scans
// the whole collection — a denial of service one request wide.
func TestLikeEscapesRegexMetacharacters(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"rock", "rock"},
		{".*", `\.\*`},
		{"(a+)+$", `\(a\+\)\+\$`},
		{"^admin", `\^admin`},
		{"a|b", `a\|b`},
		{"[a-z]", `\[a-z\]`},
		{"x{1,9999}", `x\{1,9999\}`},
		{`back\slash`, `back\\slash`},
	} {
		if got := pattern(t, tc.in).Pattern; got != tc.want {
			t.Errorf("like(%q) pattern = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The escaped term must still behave like a substring search, matching the
// characters the user typed and nothing else.
func TestLikeMatchesLiterally(t *testing.T) {
	for _, tc := range []struct {
		term, subject string
		want          bool
	}{
		{"rock", "Rock Night", true},     // case-insensitive substring
		{"a.b", "a.b arena", true},       // the dot is a literal dot
		{"a.b", "axb arena", false},      // ...so it must not match any character
		{".*", "anything at all", false}, // a wildcard is now just two characters
		{".*", "literally .* here", true},
	} {
		p := pattern(t, tc.term)
		re, err := regexp.Compile("(?i)" + p.Pattern)
		if err != nil {
			t.Fatalf("like(%q) produced an uncompilable pattern %q: %v", tc.term, p.Pattern, err)
		}
		if got := re.MatchString(tc.subject); got != tc.want {
			t.Errorf("like(%q) matching %q = %v, want %v", tc.term, tc.subject, got, tc.want)
		}
	}
}

func TestLikeIsCaseInsensitive(t *testing.T) {
	if opts := pattern(t, "rock").Options; opts != "i" {
		t.Errorf("options = %q, want \"i\"", opts)
	}
}

// A long term cannot backtrack once escaped, but it still gets compared
// against every document, so it is capped.
func TestLikeCapsTermLength(t *testing.T) {
	got := pattern(t, strings.Repeat("a", maxSearchLen*10)).Pattern
	if len(got) != maxSearchLen {
		t.Errorf("pattern length = %d, want %d", len(got), maxSearchLen)
	}
}

// Truncation slices runes, not bytes: cutting a multi-byte character in half
// would put invalid UTF-8 into the pattern.
func TestLikeTruncationKeepsValidUTF8(t *testing.T) {
	got := pattern(t, strings.Repeat("é", maxSearchLen*2)).Pattern
	if !utf8Valid(got) {
		t.Errorf("pattern is not valid UTF-8: %q", got)
	}
	if n := len([]rune(got)); n != maxSearchLen {
		t.Errorf("pattern rune count = %d, want %d", n, maxSearchLen)
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}
