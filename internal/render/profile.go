package render

import (
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/lipgloss"
	"github.com/codingconcepts/u9n/internal/estate"
)

const (
	// profileHeight is the rows the bars are drawn in and profileCell the
	// columns one hour takes. Both are fixed, so the chart comes out the same
	// shape wherever it is piped rather than reflowing with the terminal.
	profileHeight = 16
	profileCell   = 2

	// profileTableWidth is the widest a row of the table is drawn, in
	// characters, and profileBlock what it is drawn from. The table is the
	// format that survives a pipe into anything, so its bars are ascii rather
	// than blocks in a colour.
	profileTableWidth = 40
	profileBlock      = "#"

	profileHeading = "Hourly profile"
	peakMarker     = "<- peak"

	// profileMarker points out the hours the chart's tallest bars belong to,
	// which the bars can't say on their own: the chart is 16 rows tall, so an
	// hour a whisker below the peak draws to the same height as the peak and
	// reads as a tie.
	profileMarker       = "▼"
	profileBusiestLabel = "Busiest"

	// profileTick is how often an hour is labelled, far enough apart that two
	// digit labels can't run into each other at profileCell wide.
	profileTick = 3

	// rule is the baseline the bars stand on.
	rule = "─"

	baseLabel   = "Base"
	upliftLabel = "Peak"

	// emptyProfile names the bands rather than the estate, because the usual
	// way to reach it is asking for a base from a config that carries no
	// base_vcpu column, where the estate itself is far from idle.
	emptyProfile = "No vcpu to plot: the bands asked for are zero in every hour."
)

// errNoBands is returned for a chart with every band switched off, which would
// otherwise draw an empty grid and look like the estate has no demand.
var errNoBands = errors.New("nothing to draw, ask for the base band, the peak band or both")

// Bands is which parts of the hourly draw the chart shows.
type Bands struct {
	Base bool
	Peak bool
}

// band is one layer of a bar. The two are drawn from different blocks as well
// as different colours, so the split still reads on a terminal with no colour
// and in a piped file.
type band struct {
	label   string
	block   string
	colours palette
}

// The base band takes the neutral the demand curve uses on the other chart,
// since it is the floor rather than one of the sizings, and the uplift the
// hour aware purple, because the peak windows are what that sizing is built
// out of.
var (
	baseBand   = band{label: baseLabel, block: shade, colours: utilisationColours}
	upliftBand = band{label: upliftLabel, block: full, colours: hourAwareColours}
)

// ProfileTable writes what the estate draws across the day as a row an hour,
// each an ascii bar scaled against the busiest hour of the day and the vcpu
// that hour draws. The bands are summed into the one figure rather than given
// a column each, since the bar is what the row is read off and a bar drawn
// from two numbers says neither of them.
func ProfileTable(w io.Writer, clusters []estate.Cluster, bands Bands) error {
	if !bands.Base && !bands.Peak {
		return errNoBands
	}

	hours, top := profileBars(clusters, bands)

	var b strings.Builder

	b.WriteString(profileHeading)
	b.WriteString("\n\n")

	if top == 0 {
		b.WriteString(emptyProfile)
		b.WriteString("\n")

		return writeProfile(w, b.String())
	}

	peakHours := profilePeakHours(hours, top)
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)

	for hour, d := range hours {
		total := d.Total()

		value := estate.Num(total)
		if slices.Contains(peakHours, hour) {
			value += " " + peakMarker
		}

		// Writes are buffered, so the error surfaces from Flush below.
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n",
			estate.FormatHour(hour), strings.Repeat(profileBlock, profileCells(total, top)), value)
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("laying out the profile: %w", err)
	}

	return writeProfile(w, b.String())
}

// ProfileGraph writes a bar chart of what the estate draws across the day, one
// bar an hour from 00:00 to 23:00. The base band is what every cluster holds
// around the clock and the uplift band what the peak windows add on top, so the
// two stacked are the hour's whole draw.
func ProfileGraph(w io.Writer, clusters []estate.Cluster, bands Bands) error {
	if !bands.Base && !bands.Peak {
		return errNoBands
	}

	hours, top := profileBars(clusters, bands)
	busiest := profileMarkedHours(hours, top)

	var b strings.Builder

	// The chart is given a blank line top and bottom, so it doesn't start hard
	// against the prompt that ran it or end hard against the next one.
	b.WriteString("\n")
	b.WriteString(profileLegend(bands, len(busiest) > 0))
	b.WriteString("\n\n")

	if top == 0 {
		b.WriteString(emptyProfile)
		b.WriteString("\n\n")

		return writeProfile(w, b.String())
	}

	var (
		labels = profileLabels(hours, top, bands)
		gutter = len(estate.Num(top)) + 1
	)

	if len(busiest) > 0 {
		b.WriteString(profileMarkers(busiest, profileTallest(bands), gutter))
		b.WriteString("\n")
	}

	for row := profileHeight - 1; row >= 0; row-- {
		var line strings.Builder

		line.WriteString(labels[row].render(gutter))

		for _, d := range hours {
			layer, ok := profileBand(d, top, row)
			if !ok {
				line.WriteString(strings.Repeat(" ", profileCell))
				continue
			}

			line.WriteString(lipgloss.NewStyle().Foreground(layer.colours.fill).
				Render(strings.Repeat(layer.block, profileCell)))
		}

		// The hours past the last bar on this row are padding rather than
		// chart, and trailing blanks only show up as mess in a piped file.
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteString("\n")
	}

	b.WriteString(profileBaseline(gutter))
	b.WriteString("\n")
	b.WriteString(profileHourAxis(gutter))
	b.WriteString("\n\n")

	return writeProfile(w, b.String())
}

func writeProfile(w io.Writer, profile string) error {
	if _, err := io.WriteString(w, profile); err != nil {
		return fmt.Errorf("writing the profile: %w", err)
	}

	return nil
}

// profileBars is each hour's draw reduced to the bands asked for, and the
// tallest of them, which is what the y axis is scaled to. Dropping a band
// rescales the chart, so asking for the uplift alone fills the height with the
// uplift rather than leaving it squashed under a base that isn't drawn.
func profileBars(clusters []estate.Cluster, bands Bands) ([estate.HoursPerDay]estate.Demand, float64) {
	var (
		out [estate.HoursPerDay]estate.Demand
		top float64
	)

	for hour, d := range estate.HourlyDemand(clusters) {
		if !bands.Base {
			d.Base = 0
		}

		if !bands.Peak {
			d.Uplift = 0
		}

		out[hour] = d
		top = max(top, d.Total())
	}

	return out, top
}

// profileBand is the layer one row of one bar is drawn from. The base sits at
// the foot of the bar and the uplift stacks on top of it, so the two together
// reach the hour's whole draw.
func profileBand(d estate.Demand, top float64, row int) (band, bool) {
	switch {
	case row < barRows(d.Base, top):
		return baseBand, true

	case row < barRows(d.Total(), top):
		return upliftBand, true
	}

	return band{}, false
}

// barRows is how many rows a value fills, rounded up so any demand at all
// takes a row rather than disappearing off the foot of the chart.
//
// The top row is kept for the busiest hours alone. Rounding up would otherwise
// carry every hour within a sixteenth of the peak into it, and a chart whose
// tallest bar is drawn eleven times over says the day has no busiest hour.
func barRows(value, top float64) int {
	switch {
	case value <= 0:
		return 0
	case value >= top:
		return profileHeight
	}

	return min(max(int(math.Ceil(value/top*profileHeight)), 1), profileHeight-1)
}

// profileCells is the same for the table's rows, which are drawn across the
// page rather than up it, and which keep their last character for the busiest
// hours for the same reason.
func profileCells(value, top float64) int {
	switch {
	case value <= 0:
		return 0
	case value >= top:
		return profileTableWidth
	}

	return min(max(int(math.Ceil(value/top*profileTableWidth)), 1), profileTableWidth-1)
}

// profilePeakHours are the hours the bands drawn are tallest in, which is the
// row the rest of the day is scaled against. Every hour tied for the top is
// marked rather than only the first of them, and the ties are exact by
// construction: two hours the same clusters draw the same vcpu in are summed
// from the same numbers in the same order, so they come out bit for bit
// identical rather than merely close.
func profilePeakHours(hours [estate.HoursPerDay]estate.Demand, top float64) []int {
	if top == 0 {
		return nil
	}

	var out []int

	for hour, d := range hours {
		if d.Total() == top {
			out = append(out, hour)
		}
	}

	return out
}

// profileMarkedHours are the hours the chart points at. A day whose every hour
// is the busiest has no busiest hour to pick out of it, so it is left unmarked
// rather than given a pointer over all 24.
func profileMarkedHours(hours [estate.HoursPerDay]estate.Demand, top float64) []int {
	out := profilePeakHours(hours, top)
	if len(out) == estate.HoursPerDay {
		return nil
	}

	return out
}

// profileLabels pairs rows with the numbers printed against them: the tallest
// hour of the chart, and the base band's ceiling where both bands are drawn.
// The base is the same in all 24 hours, so one label covers it.
//
// The foot is only labelled zero when no band reaches it, since a number
// against a row the bars are drawn in should say what that row holds rather
// than what it was measured from.
func profileLabels(hours [estate.HoursPerDay]estate.Demand, top float64, bands Bands) map[int]axisLabel {
	labels := map[int]axisLabel{}

	labels[barRows(top, top)-1] = axisLabel{text: estate.Num(top), colour: profileTallest(bands).colours.label}

	// The base earns its own number only when an uplift is stacked above it,
	// and not when it already carries the tallest bar's label.
	if base := hours[0].Base; bands.Base && bands.Peak && base > 0 && base < top {
		row := barRows(base, top) - 1
		if _, taken := labels[row]; !taken {
			labels[row] = axisLabel{text: estate.Num(base), colour: baseBand.colours.label}
		}
	}

	return labels
}

// profileTallest is the band the top of the chart belongs to, which is the
// uplift wherever it is drawn and the base when it isn't. The axis number and
// the markers take their colour from it, so both read as belonging to the bars
// they are set against.
func profileTallest(bands Bands) band {
	if bands.Peak {
		return upliftBand
	}

	return baseBand
}

// profileMarkers is the row of pointers over the chart, one over each hour the
// estate is busiest in, set at the left of the hour's cell so it lines up with
// the hour's label on the axis below.
func profileMarkers(hours []int, tallest band, gutter int) string {
	var b strings.Builder

	b.WriteString(strings.Repeat(" ", gutter))

	for hour := range estate.HoursPerDay {
		if !slices.Contains(hours, hour) {
			b.WriteString(strings.Repeat(" ", profileCell))
			continue
		}

		b.WriteString(lipgloss.NewStyle().Foreground(tallest.colours.fill).Render(profileMarker))
		b.WriteString(strings.Repeat(" ", profileCell-1))
	}

	return strings.TrimRight(b.String(), " ")
}

// profileLegend keys the bands that are drawn, in the order they are stacked,
// and the marker when the day has a busiest hour to point at.
func profileLegend(bands Bands, marked bool) string {
	var legend []string

	if bands.Base {
		legend = append(legend, legendEntry(baseBand))
	}

	if bands.Peak {
		legend = append(legend, legendEntry(upliftBand))
	}

	if marked {
		marker := lipgloss.NewStyle().Foreground(profileTallest(bands).colours.fill).Render(profileMarker)
		legend = append(legend, fmt.Sprintf("%s %s", marker, profileBusiestLabel))
	}

	return strings.Join(legend, legendGap)
}

func legendEntry(b band) string {
	swatch := lipgloss.NewStyle().Foreground(b.colours.fill).Render(strings.Repeat(b.block, 2))

	return fmt.Sprintf("%s %s", swatch, b.label)
}

// profileBaseline is the rule the bars stand on, which is where the zero on
// the axis belongs. Every row above it holds part of a bar, so a zero printed
// against one of those would name a row that is anything but.
func profileBaseline(gutter int) string {
	return axisLabel{text: "0"}.render(gutter) + strings.Repeat(rule, estate.HoursPerDay*profileCell)
}

// profileHourAxis labels every third hour, which is as often as two digits fit
// without the labels touching.
func profileHourAxis(gutter int) string {
	var b strings.Builder

	b.WriteString(strings.Repeat(" ", gutter))

	for hour := 0; hour < estate.HoursPerDay; hour += profileTick {
		text := fmt.Sprintf("%02d", hour)

		b.WriteString(text)
		b.WriteString(strings.Repeat(" ", max(profileCell*profileTick-len(text), 0)))
	}

	return strings.TrimRight(b.String(), " ")
}
