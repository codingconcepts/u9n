package text

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWrap(t *testing.T) {
	cases := []struct {
		name  string
		given string
		width int
		exp   string
	}{
		{
			name:  "breaks on the space before the column",
			given: "one two three four",
			width: 7,
			exp:   "one two\nthree\nfour",
		},
		{
			name:  "a line that fits is left alone",
			given: "one two three",
			width: 20,
			exp:   "one two three",
		},
		{
			// The width a string is written to in the source says nothing
			// about the width it is printed at.
			name:  "line breaks within a paragraph are rewrapped",
			given: "one\ntwo\nthree four",
			width: 9,
			exp:   "one two\nthree\nfour",
		},
		{
			name:  "blank lines keep paragraphs apart",
			given: "one two\n\nthree four",
			width: 7,
			exp:   "one two\n\nthree\nfour",
		},
		{
			// A word wider than the column overhangs rather than being cut in
			// two, since the long words are figures and paths.
			name:  "a word wider than the column is left whole",
			given: "a 1,234,567 b",
			width: 5,
			exp:   "a\n1,234,567\nb",
		},
		{
			name:  "runs of whitespace come out as single spaces",
			given: "  one   two  ",
			width: 20,
			exp:   "one two",
		},
		{
			name:  "nothing to wrap",
			given: "",
			width: 10,
			exp:   "",
		},
		{
			// Width is counted in characters rather than bytes, so a line of
			// them doesn't wrap short of the column.
			name:  "multi byte characters count as one column",
			given: "é é é é",
			width: 5,
			exp:   "é é é\né",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Wrap(c.given, c.width); got != c.exp {
				t.Errorf("expected:\n%q\ngot:\n%q", c.exp, got)
			}
		})
	}
}

// Nothing the commands print is wider than the column it is set in, which is
// the whole point of wrapping it there.
func TestWrapStaysWithinTheWidth(t *testing.T) {
	const given = "Size the estate three ways: as it stands, assuming every cluster peaks at " +
		"once, and walking the 24 hour clock."

	for line := range strings.SplitSeq(Wrap(given, Width), "\n") {
		if got := utf8.RuneCountInString(line); got > Width {
			t.Errorf("expected no line wider than %d, got %d: %q", Width, got, line)
		}
	}
}
