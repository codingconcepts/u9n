package render

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codingconcepts/u9n/internal/estate"
)

const (
	peakBarWidth = 40

	peakGap          = "  "
	avoidedLabel     = "Avoided"
	sumOfPeaksLabel  = "Sum of the peaks"
	peakOfSumLabel   = "Peak of the sums"
	totalLabel       = "Total"
	sumOfPeaksColumn = "Sum of peaks"
	peakOfSumsColumn = "Peak of sums"
	peakHeading      = "Every cluster at its own peak, versus what they draw together at %s"
	anyHour          = "any hour of the day"
	noWindows        = "No peak windows in the config, so every cluster counts as at peak in every hour and there is nothing to avoid."
	flatDay          = "The estate draws the same in every hour, so every hour is its peak."
	emptyPeak        = "No vcpu to plot: the estate draws nothing in any hour."
	spreadHeading    = "What each sizing holds idle, hour by hour (excluding headroom)"

	spreadHoursListed = 4
	hoursCount        = "%d hours"

	busiestLabel       = "Busiest"
	averageLabel       = "Average"
	quietestLabel      = "Quietest"
	drawnColumn        = "Drawn"
	idleSumOfPeaks     = "Idle sum of peaks"
	idlePeakOfSums     = "Idle peak of sums"
	hoursColumn        = "Hours"
	idleWithPercentage = "%s (%s)"
)

// The kept band is what the hour aware sizing stands up and the avoided band
// what only the worst case pays for, so each takes the colour its sizing is
// drawn in on the other charts.
// The sum of the peaks is a sizing in its own right rather than a split of
// one, so it is drawn whole in the worst case colour instead of as the two
// bands it would break into.
var (
	keptBand    = band{block: full, colours: hourAwareColours}
	avoidedBand = band{block: shade, colours: worstCaseColours}
	sumBand     = band{block: full, colours: worstCaseColours}
)

// Peak writes a chart of the two ways the estate can be sized: for the sum of
// every cluster's peak, and for the peak of what they draw together. The gap
// between them is the capacity bought for peaks that never land in the same
// hour, which is the whole case for consolidating.
func Peak(w io.Writer, s estate.Summary) error {
	var b strings.Builder

	// The chart is given a blank line top and bottom, so it doesn't start hard
	// against the prompt that ran it or end hard against the next one.
	b.WriteString("\n")

	if s.WorstCase.Peak == 0 {
		b.WriteString(emptyPeak)
		b.WriteString("\n\n")

		return writePeak(w, b.String())
	}

	hour := s.HourAware.PeakHours[0]

	b.WriteString(peakTotals(s))
	b.WriteString("\n")
	fmt.Fprintf(&b, peakHeading+"\n\n", peakHeadingHours(s.HourAware.PeakHours))
	b.WriteString(peakClusters(s, hour))

	// The notes come before the spread, since what they have to say is about
	// the rows the chart was read off rather than about the day they were
	// taken out of.
	for _, note := range peakNotes(s) {
		b.WriteString("\n")
		b.WriteString(note)
		b.WriteString("\n")
	}

	if spread := peakSpread(s); spread != "" {
		b.WriteString("\n")
		b.WriteString(spreadHeading)
		b.WriteString("\n\n")
		b.WriteString(spread)
	}

	b.WriteString("\n")

	return writePeak(w, b.String())
}

func writePeak(w io.Writer, chart string) error {
	if _, err := io.WriteString(w, chart); err != nil {
		return fmt.Errorf("writing the peak comparison: %w", err)
	}

	return nil
}

// segment is a run of one band inside a bar, since a bar is sometimes a
// sizing of its own and sometimes a sizing split into the parts it is made of.
type segment struct {
	band  band
	cells int
}

// peakTotals is the three rows the whole estate comes down to: the two
// sizings, and the difference between them. The lower two are the upper one
// taken apart, so they line up under the length they are a split of, with the
// sum of the peaks left whole rather than drawn as the parts it breaks into.
func peakTotals(s estate.Summary) string {
	kept, avoided := peakCells(s.HourAware.Peak, s.WorstCase.Peak, s.WorstCase.Peak)

	var (
		labels = []string{sumOfPeaksLabel, peakOfSumLabel, avoidedLabel}
		rows   = []struct {
			indent   int
			segments []segment
			value    string
		}{
			{
				segments: []segment{{sumBand, kept + avoided}},
				value:    estate.Num(s.WorstCase.Peak),
			},
			{
				segments: []segment{{keptBand, kept}},
				value:    estate.Num(s.HourAware.Peak),
			},
			{
				indent:   kept,
				segments: []segment{{avoidedBand, avoided}},
				value:    fmt.Sprintf("%s (%s)", estate.Num(s.WorstCase.Peak-s.HourAware.Peak), percent(s.Diversity())),
			},
		}
	)

	width := longest(labels)

	var b strings.Builder

	for i, row := range rows {
		b.WriteString(padRight(labels[i], width))
		b.WriteString(peakGap)
		b.WriteString(strings.Repeat(" ", row.indent))

		drawn := row.indent

		for _, segment := range row.segments {
			b.WriteString(bar(segment.band, segment.cells))
			drawn += segment.cells
		}

		b.WriteString(strings.Repeat(" ", peakBarWidth-drawn))
		b.WriteString(peakGap)
		b.WriteString(row.value)
		b.WriteString("\n\n")
	}

	return b.String()
}

// peakClusters is one row a cluster, busiest first, each drawn as its own peak
// with the part of it the estate is actually serving at the peak hour filled
// in. The solid parts add up to the peak of the sum and the whole bars to the
// sum of the peaks, so the rows account for the gap between the two.
func peakClusters(s estate.Summary, hour int) string {
	clusters := slices.SortedStableFunc(slices.Values(s.Clusters), func(a, b estate.Cluster) int {
		return cmp.Or(cmp.Compare(b.Peak, a.Peak), cmp.Compare(a.Name, b.Name))
	})

	// The bars are scaled against the biggest cluster rather than the estate,
	// since a bar an eightieth of the estate long says nothing about the
	// cluster it stands for.
	var top float64
	for _, c := range clusters {
		top = max(top, c.Peak)
	}

	var (
		names    = make([]string, 0, len(clusters)+1)
		atHour   = make([]string, 0, len(clusters)+1)
		atPeak   = make([]string, 0, len(clusters)+1)
		barCells = make([][2]int, 0, len(clusters))
	)

	for _, c := range clusters {
		kept, avoided := peakCells(c.VCPUAt(hour), c.Peak, top)

		names = append(names, c.Name)
		atHour = append(atHour, estate.Num(c.VCPUAt(hour)))
		atPeak = append(atPeak, estate.Num(c.Peak))
		barCells = append(barCells, [2]int{kept, avoided})
	}

	var (
		nameWidth = longest(append(names, totalLabel))
		hourWidth = longest(append(atHour, peakOfSumsColumn))
		peakWidth = longest(append(atPeak, sumOfPeaksColumn))
	)

	var b strings.Builder

	// The column headings sit over the figures rather than over the bars,
	// which carry their own key in the legend. They are ordered the way the
	// totals above them are, with what the cluster adds to the sum of the
	// peaks first and what it adds to the peak of the sum beside it.
	b.WriteString(strings.Repeat(" ", nameWidth+len(peakGap)+peakBarWidth))
	b.WriteString(peakGap)
	b.WriteString(padLeft(sumOfPeaksColumn, peakWidth))
	b.WriteString(peakGap)
	b.WriteString(padLeft(peakOfSumsColumn, hourWidth))
	b.WriteString("\n")

	for i := range clusters {
		cells := barCells[i]

		b.WriteString(padRight(names[i], nameWidth))
		b.WriteString(peakGap)
		b.WriteString(bar(keptBand, cells[0]))
		b.WriteString(bar(avoidedBand, cells[1]))
		b.WriteString(strings.Repeat(" ", peakBarWidth-cells[0]-cells[1]))
		b.WriteString(peakGap)
		b.WriteString(padLeft(atPeak[i], peakWidth))
		b.WriteString(peakGap)
		b.WriteString(padLeft(atHour[i], hourWidth))
		b.WriteString("\n")
	}

	b.WriteString(padRight(totalLabel, nameWidth+len(peakGap)+peakBarWidth))
	b.WriteString(peakGap)
	b.WriteString(padLeft(estate.Num(s.WorstCase.Peak), peakWidth))
	b.WriteString(peakGap)
	b.WriteString(padLeft(estate.Num(s.HourAware.Peak), hourWidth))
	b.WriteString("\n")

	return b.String()
}

// peakHeadingHours names every hour the estate holds its peak for, since the
// comparison the heading draws holds in all of them rather than only in the
// one the rows were read off. A day that peaks in every hour is named rather
// than listed, both because the list would be the whole clock and because
// "all day" doesn't follow the "draws at" the heading reads into it.
func peakHeadingHours(hours []int) string {
	if len(hours) == estate.HoursPerDay {
		return anyHour
	}

	return estate.FormatHours(hours)
}

// peakSpread reads the chart's comparison across the whole day rather than in
// the one hour the rows above were taken from, and against both sizings
// rather than only the worst case. The busiest hour is the hour either rack
// comes closest to being justified in, so the gaps it shows are the smallest
// the day has: the average and the quietest hour are what the same racks hold
// the rest of the time.
//
// It is nothing at all for a day with no shape to read, since three rows of
// the same figures say less than the one row above them already did.
func peakSpread(s estate.Summary) string {
	peakHours, quietHours := s.HourAware.PeakHours, s.QuietHours()

	drawn := [...]float64{
		s.Hourly[peakHours[0]],
		s.VCPUHours() / estate.HoursPerDay,
		s.Hourly[quietHours[0]],
	}

	if drawn[0] == drawn[2] {
		return ""
	}

	var (
		labels    = []string{busiestLabel, averageLabel, quietestLabel}
		hours     = []string{spreadHours(peakHours), estate.AllDay, spreadHours(quietHours)}
		drawns    = make([]string, 0, len(drawn))
		idleSums  = make([]string, 0, len(drawn))
		idlePeaks = make([]string, 0, len(drawn))
	)

	for _, value := range drawn {
		// The average is the only row that lands between whole vcpu, and a
		// fraction of a core is below what any of this is accurate to. The
		// idle columns are taken off the rounded figure rather than rounded in
		// their own right, so each of them still adds up to its rack.
		value = math.Round(value)

		drawns = append(drawns, estate.Num(value))
		idleSums = append(idleSums, idle(s.WorstCase.Peak, value))
		idlePeaks = append(idlePeaks, idle(s.HourAware.Peak, value))
	}

	var (
		labelWidth    = longest(labels)
		hoursWidth    = longest(append(hours, hoursColumn))
		drawnWidth    = longest(append(drawns, drawnColumn))
		idleSumWidth  = longest(append(idleSums, idleSumOfPeaks))
		idlePeakWidth = longest(append(idlePeaks, idlePeakOfSums))
	)

	var b strings.Builder

	b.WriteString(strings.Repeat(" ", labelWidth))
	b.WriteString(peakGap)
	b.WriteString(padRight(hoursColumn, hoursWidth))
	b.WriteString(peakGap)
	b.WriteString(padLeft(drawnColumn, drawnWidth))
	b.WriteString(peakGap)
	b.WriteString(padLeft(idleSumOfPeaks, idleSumWidth))
	b.WriteString(peakGap)
	b.WriteString(padLeft(idlePeakOfSums, idlePeakWidth))
	b.WriteString("\n")

	for i := range labels {
		b.WriteString(padRight(labels[i], labelWidth))
		b.WriteString(peakGap)
		b.WriteString(padRight(hours[i], hoursWidth))
		b.WriteString(peakGap)
		b.WriteString(padLeft(drawns[i], drawnWidth))
		b.WriteString(peakGap)
		b.WriteString(padLeft(idleSums[i], idleSumWidth))
		b.WriteString(peakGap)
		b.WriteString(padLeft(idlePeaks[i], idlePeakWidth))
		b.WriteString("\n")
	}

	return b.String()
}

// idle is what a rack holds that nothing is drawing on, as vcpu and as the
// share of the rack they are. The two are given together, since the vcpu says
// how much capacity is standing idle and the share says how badly the sizing
// fits the hour.
func idle(rack, drawn float64) string {
	var share float64
	if rack != 0 {
		share = (rack - drawn) / rack * 100
	}

	return fmt.Sprintf(idleWithPercentage, estate.Num(rack-drawn), percent(share))
}

// spreadHours labels the hours a spread row covers. A run too long to read as
// a list is counted instead, since what a row of 22 hours says is how much of
// the day sits there rather than which hours they are.
func spreadHours(hours []int) string {
	switch {
	case len(hours) == estate.HoursPerDay:
		return estate.AllDay

	case len(hours) > spreadHoursListed:
		return fmt.Sprintf(hoursCount, len(hours))
	}

	return estate.FormatHours(hours)
}

// peakNotes are the footnotes the chart needs to be read honestly: one for a
// config the comparison can't say anything about, and one for a day with no
// peak to pick out of it.
func peakNotes(s estate.Summary) []string {
	switch {
	case !s.Windowed:
		return []string{noWindows}

	case len(s.HourAware.PeakHours) == estate.HoursPerDay:
		return []string{flatDay}
	}

	return nil
}

// peakCells splits a bar between the vcpu drawn at the peak hour and the vcpu
// only the sum of the peaks pays for. Either part keeps a cell of its own
// wherever it is anything at all, so a sliver reads as a sliver rather than as
// nothing, and the two together never run past the bar.
func peakCells(kept, total, top float64) (int, int) {
	cells := barCells(total, top)

	// A bar split in two needs a cell for each part, however little of the
	// estate the cluster is, or the split it was drawn to show is lost.
	if kept > 0 && total > kept {
		cells = max(cells, 2)
	}

	keptCells := min(barCells(kept, top), cells)

	if total > kept && keptCells == cells && cells > 0 {
		keptCells--
	}

	return keptCells, cells - keptCells
}

// barCells is the cells a value fills, rounded up so any vcpu at all takes a
// cell rather than disappearing off the end of the bar.
func barCells(value, top float64) int {
	if value <= 0 {
		return 0
	}

	return min(max(int(math.Ceil(value/top*peakBarWidth)), 1), peakBarWidth)
}

// bar draws a run of one band's block, which is nothing at all for a band with
// no cells rather than an empty escape sequence.
func bar(b band, cells int) string {
	if cells <= 0 {
		return ""
	}

	return lipgloss.NewStyle().Foreground(b.colours.fill).Render(strings.Repeat(b.block, cells))
}

// longest is the width the widest of the strings needs.
func longest(values []string) int {
	var width int
	for _, value := range values {
		width = max(width, len(value))
	}

	return width
}

func padRight(text string, width int) string {
	return text + strings.Repeat(" ", max(width-len(text), 0))
}

func padLeft(text string, width int) string {
	return strings.Repeat(" ", max(width-len(text), 0)) + text
}
