package render

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codingconcepts/u9n/internal/estate"
)

const (
	// profileHeight is the rows the bars are drawn in and profileCell the
	// columns one hour takes. Both are fixed, so the chart comes out the same
	// shape wherever it is piped rather than reflowing with the terminal.
	profileHeight = 16
	profileCell   = 2

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

// Profile writes a bar chart of what the estate draws across the day, one bar
// an hour from 00:00 to 23:00. The base band is what every cluster holds around
// the clock and the uplift band what the peak windows add on top, so the two
// stacked are the hour's whole draw.
func Profile(w io.Writer, clusters []estate.Cluster, bands Bands) error {
	if !bands.Base && !bands.Peak {
		return errNoBands
	}

	hours, top := profileBars(clusters, bands)

	var b strings.Builder

	// The chart is given a blank line top and bottom, so it doesn't start hard
	// against the prompt that ran it or end hard against the next one.
	b.WriteString("\n")
	b.WriteString(profileLegend(bands))
	b.WriteString("\n\n")

	if top == 0 {
		b.WriteString(emptyProfile)
		b.WriteString("\n\n")

		if _, err := io.WriteString(w, b.String()); err != nil {
			return fmt.Errorf("writing the profile: %w", err)
		}

		return nil
	}

	var (
		labels = profileLabels(hours, top, bands)
		gutter = len(estate.Num(top)) + 1
	)

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

	if _, err := io.WriteString(w, b.String()); err != nil {
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
func barRows(value, top float64) int {
	if value <= 0 {
		return 0
	}

	return min(max(int(math.Ceil(value/top*profileHeight)), 1), profileHeight)
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

	tallest := upliftBand
	if !bands.Peak {
		tallest = baseBand
	}

	labels[barRows(top, top)-1] = axisLabel{text: estate.Num(top), colour: tallest.colours.label}

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

// profileLegend keys the bands that are drawn, in the order they are stacked.
func profileLegend(bands Bands) string {
	var legend []string

	if bands.Base {
		legend = append(legend, legendEntry(baseBand))
	}

	if bands.Peak {
		legend = append(legend, legendEntry(upliftBand))
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
