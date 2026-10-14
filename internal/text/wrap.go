// Package text lays out the prose the commands and the charts print, so the
// width it is set in is a number in one place rather than a newline in the
// middle of every string.
package text

import (
	"strings"
	"unicode/utf8"
)

// Width is the column prose is wrapped at. It is the width the help and the
// notes were already written to by hand, which sits inside an eighty column
// terminal with room for the quoting a pager or a diff puts in front of it.
const Width = 65

// paragraph is the break between one paragraph and the next, which is the one
// line break in a string that means something.
const paragraph = "\n\n"

// Wrap breaks s into lines no wider than width, on the spaces between words.
//
// Blank lines are kept, since they separate paragraphs. Every other line break
// is read as a space, so a string can be written to whatever width suits the
// source it sits in and still come out at the width it is printed at. Values
// are substituted into a string before it is wrapped, so a long figure pushes
// the words after it along rather than overhanging the column.
//
// A word longer than the width is left to overhang on a line of its own rather
// than broken, since the words that long are numbers, paths and flags, all of
// which are harder to read split in two.
func Wrap(s string, width int) string {
	paragraphs := strings.Split(s, paragraph)

	for i, p := range paragraphs {
		paragraphs[i] = wrapParagraph(p, width)
	}

	return strings.Join(paragraphs, paragraph)
}

// wrapParagraph is Wrap over a run of text with no blank line in it, which is
// where the words are actually laid out.
func wrapParagraph(p string, width int) string {
	words := strings.Fields(p)
	if len(words) == 0 {
		return ""
	}

	var (
		b    strings.Builder
		line int
	)

	for i, w := range words {
		n := utf8.RuneCountInString(w)

		switch {
		// The first word of a paragraph starts the line whatever its length,
		// so a word wider than the column overhangs rather than leaving a
		// blank line above itself.
		case i == 0:
			b.WriteString(w)

			line = n

		case line+1+n > width:
			b.WriteString("\n")
			b.WriteString(w)

			line = n

		default:
			b.WriteString(" ")
			b.WriteString(w)

			line += 1 + n
		}
	}

	return b.String()
}
