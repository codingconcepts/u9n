package render

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/codingconcepts/u9n/internal/estate"
	"github.com/codingconcepts/u9n/internal/text"
)

const (
	compareHeading = "Every cluster on hardware of its own, showing its base and peak"
	compareTotal   = "Total - every peak provisioned at once"

	// compareTogether heads the same total taken at the hour the clusters are
	// busiest together, which is the sizing the estate needs when they share
	// hardware rather than each standing up a peak of its own.
	compareTogether = "Total - the most they ever draw at once (%s)"

	togetherLabel = "Together"

	// compareSeparator keeps the total's segments apart, so the row still
	// reads as the bars above it laid end to end on a terminal with no colour
	// and in a piped file.
	compareSeparator = " "

	// compareNote names the gap between the two totals, which the bars show
	// as a difference in length but can't put a figure on. It is wrapped once
	// the figures are in it, so a long one pushes the words along rather than
	// running past the column the rest of the prose is set in.
	compareNote = "The %s (%s) between them is racked for peaks that never land in the same hour. The peak command charts the gap."

	emptyCompare = "No vcpu to plot: every cluster peaks at nothing."
)

// The base takes the neutral the other charts draw their floor in, and the
// peak the green of the sum of the peaks, since the total these bars add up to
// is that sizing itself. What the clusters draw together takes the purple of
// the peak of the sum for the same reason: the total that row comes to is that
// sizing.
var (
	compareBaseBand     = band{label: baseLabel, block: shade, colours: utilisationColours}
	comparePeakBand     = band{label: upliftLabel, block: full, colours: sumOfPeaksColours}
	compareTogetherBand = band{label: togetherLabel, block: full, colours: peakOfSumColours}
)

// Compare writes the estate as it is racked when every cluster gets hardware
// of its own: a cluster at a time, each drawn from the base it holds all day
// up to the peak its own hardware has to cover, and then the total those peaks
// come to.
//
// Every bar is drawn to one scale, the sum of the peaks, so the total is the
// bars above it laid end to end rather than a bar in its own right. This is
// demand rather than a sizing, so it carries no headroom or growth.
func Compare(w io.Writer, s estate.Summary) error {
	var b strings.Builder

	// The chart is given a blank line top and bottom, so it doesn't start hard
	// against the prompt that ran it or end hard against the next one.
	b.WriteString("\n")

	if s.SumOfPeaks.Peak == 0 {
		b.WriteString(emptyCompare)
		b.WriteString("\n\n")

		return writeCompare(w, b.String())
	}

	// Biggest first, so the clusters the total is mostly made of are read
	// first and the segments of the total run in the same order as the rows.
	clusters := slices.SortedStableFunc(slices.Values(s.Clusters), func(a, b estate.Cluster) int {
		return cmp.Or(cmp.Compare(b.Peak, a.Peak), cmp.Compare(a.Name, b.Name))
	})

	var (
		cells  = compareCells(clusters, s.SumOfPeaks.Peak)
		blocks = compareBlocks(cells)
		width  = compareWidth(cells)
		value  = compareValueWidth(clusters, s.SumOfPeaks.Peak)
	)

	// The hour the clusters are busiest together, and whether it is worth
	// drawing. Without peak windows every cluster counts as at peak in every
	// hour, so the two totals are the same bar twice over.
	var (
		hour     = s.PeakOfSum.PeakHours[0]
		together = s.Windowed && s.SumOfPeaks.Peak != s.PeakOfSum.Peak
	)

	legend := []string{legendEntry(compareBaseBand), legendEntry(comparePeakBand)}
	if together {
		legend = append(legend, legendEntry(compareTogetherBand))
	}

	b.WriteString(compareHeading)
	b.WriteString("\n\n")
	b.WriteString(strings.Join(legend, legendGap))
	b.WriteString("\n\n")

	for i, c := range clusters {
		// The base is scaled the same way the peak is and capped by it, so the
		// shaded bar reads as the part of the solid one above it the cluster
		// holds whether it is working or not.
		base := min(compareScaled(c.Base, s.SumOfPeaks.Peak, blocks), cells[i])

		b.WriteString(c.Name)
		b.WriteString("\n")
		b.WriteString(compareRow(compareBaseBand, base, width, estate.Num(c.Base), value))
		b.WriteString(compareRow(comparePeakBand, cells[i], width, estate.Num(c.Peak), value))
		b.WriteString("\n")
	}

	b.WriteString(compareTotal)
	b.WriteString("\n\n")
	b.WriteString(compareTotalBar(cells, comparePeakBand, width, estate.Num(s.SumOfPeaks.Peak), value))

	// The same total taken at the hour the clusters share, segment for
	// segment, so the length the bar loses against the one above it is the
	// capacity shared hardware never has to stand up.
	if together {
		b.WriteString("\n")
		fmt.Fprintf(&b, compareTogether+"\n\n", peakHeadingHours(s.PeakOfSum.PeakHours))
		b.WriteString(compareTotalBar(compareHourCells(clusters, cells, hour),
			compareTogetherBand, width, estate.Num(s.PeakOfSum.Peak), value))
	}

	if note := compareNotes(s); note != "" {
		b.WriteString("\n")
		b.WriteString(note)
		b.WriteString("\n\n")
	}

	return writeCompare(w, b.String())
}

func writeCompare(w io.Writer, chart string) error {
	if _, err := io.WriteString(w, chart); err != nil {
		return fmt.Errorf("writing the comparison: %w", err)
	}

	return nil
}

// compareRow is one bar and the vcpu it stands for, the bar padded out to the
// width of the total below it so every figure lands in the same column.
func compareRow(bd band, cells, width int, value string, valueWidth int) string {
	return bar(bd, cells) +
		strings.Repeat(" ", max(width-cells, 0)) +
		peakGap +
		padLeft(value, valueWidth) +
		"\n"
}

// compareTotalBar is the clusters' bars laid end to end, which is what makes
// the row read as those rows added up rather than as a bar of its own. Only
// the total is printed against it: the clusters carry their own figures above,
// and a column of them over the segments is a row of numbers to be read rather
// than a bar.
//
// The segments run longest first whatever order the clusters came in, so the
// row steps down evenly. The rows above are already in that order, but the
// hour the clusters share isn't: a cluster with a big peak sitting at its base
// draws less in that hour than a smaller one inside its window.
func compareTotalBar(cells []int, bd band, width int, total string, valueWidth int) string {
	var (
		bars  strings.Builder
		start int
	)

	for _, c := range slices.SortedFunc(slices.Values(cells), func(a, b int) int {
		return cmp.Compare(b, a)
	}) {
		if c == 0 {
			continue
		}

		if start > 0 {
			bars.WriteString(compareSeparator)

			start++
		}

		bars.WriteString(bar(bd, c))

		start += c
	}

	return bars.String() +
		strings.Repeat(" ", max(width-start, 0)) +
		peakGap +
		padLeft(total, valueWidth) +
		"\n"
}

// compareHourCells is the segments of the total cut back to what each cluster
// draws in the hour they are busiest together: its own peak where its window
// covers the hour, and the base it holds all day where it doesn't.
//
// A segment is cut out of the cells the cluster's peak was drawn with rather
// than scaled afresh, so the two totals can be read against each other
// segment by segment as well as end to end. A cluster drawing anything at all
// keeps a cell, for the same reason it keeps one above.
func compareHourCells(clusters []estate.Cluster, cells []int, hour int) []int {
	out := make([]int, len(clusters))

	for i, c := range clusters {
		drawn := c.VCPUAt(hour)
		if drawn <= 0 || c.Peak <= 0 {
			continue
		}

		out[i] = min(max(int(math.Round(drawn/c.Peak*float64(cells[i]))), 1), cells[i])
	}

	return out
}

// compareCells divides the bar between the clusters by their share of the sum
// of the peaks. The separators the total needs between its segments are taken
// out of the bar first, so a row of segments comes to the same width as the
// bars above it however many clusters there are, and the bar is only allowed
// past that width when there are more clusters than a cell each would fit in.
func compareCells(clusters []estate.Cluster, total float64) []int {
	width := max(peakBarWidth-(len(clusters)-1), len(clusters))

	var (
		cells = make([]int, len(clusters))
		rems  = make([]float64, len(clusters))
		drawn []int
	)

	// A cluster with any peak at all keeps a cell of its own, so a cluster an
	// eightieth of the estate reads as a sliver rather than as nothing. A
	// cluster that peaks at nothing needs no hardware, so it gets no segment.
	for i, c := range clusters {
		if c.Peak > 0 {
			cells[i] = 1

			drawn = append(drawn, i)
		}
	}

	// The cells those leave are shared out by what each cluster is of the
	// total, rounded down, which leaves a handful of them over.
	share := width - len(drawn)
	if len(drawn) == 0 || share <= 0 {
		return cells
	}

	left := share

	for _, i := range drawn {
		exact := clusters[i].Peak / total * float64(share)

		cells[i] += int(exact)
		rems[i] = exact - math.Trunc(exact)
		left -= int(exact)
	}

	// The ones left over go to the clusters the rounding shortchanged most, so
	// the segments add up to the bar exactly rather than to a column either
	// side of it. Clusters rounded by the same amount are served in the order
	// they are drawn in, which is biggest first.
	order := slices.SortedStableFunc(slices.Values(drawn), func(a, b int) int {
		return cmp.Compare(rems[b], rems[a])
	})

	for i := 0; left > 0; i, left = i+1, left-1 {
		cells[order[i%len(order)]]++
	}

	return cells
}

// compareBlocks is the cells the bars themselves are drawn from, which is the
// scale every value on the chart is measured against.
func compareBlocks(cells []int) int {
	var total int
	for _, c := range cells {
		total += c
	}

	return total
}

// compareWidth is the columns the total takes up, which is its blocks plus the
// separators between them. Every other bar is padded out to it, so the figures
// all land in the same column.
func compareWidth(cells []int) int {
	var drawn int

	for _, c := range cells {
		if c > 0 {
			drawn++
		}
	}

	return compareBlocks(cells) + max(drawn-1, 0)
}

// compareValueWidth is the column the vcpu figures are set in, wide enough for
// the longest of them, which is usually the total the rest add up to.
func compareValueWidth(clusters []estate.Cluster, total float64) int {
	values := make([]string, 0, len(clusters)*2+1)
	values = append(values, estate.Num(total))

	for _, c := range clusters {
		values = append(values, estate.Num(c.Base), estate.Num(c.Peak))
	}

	return longest(values)
}

// compareNotes reads the total back against the hour the clusters share, which
// is the one thing a chart of separate hardware can't show. There is nothing
// to say for a config with no peak windows, where every cluster counts as at
// peak in every hour and the two numbers are the same.
func compareNotes(s estate.Summary) string {
	if !s.Windowed || s.SumOfPeaks.Peak == s.PeakOfSum.Peak {
		return ""
	}

	return text.Wrap(fmt.Sprintf(compareNote,
		estate.Num(s.SumOfPeaks.Peak-s.PeakOfSum.Peak),
		percent(s.Diversity())), text.Width)
}

// compareScaled is the cells a value fills on the shared scale, rounded up so
// any vcpu at all takes a cell rather than disappearing off the end of the bar.
func compareScaled(value, total float64, width int) int {
	if value <= 0 {
		return 0
	}

	return min(max(int(math.Ceil(value/total*float64(width))), 1), width)
}
