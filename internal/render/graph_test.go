package render

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/codingconcepts/u9n/internal/estate"
)

// graphFixture is two clusters peaking in different windows, so the peak of
// the sum line moves and the three sizings differ.
func graphFixture() estate.Summary {
	return estate.Summarise([]estate.Cluster{
		{Name: "a", Provisioned: 100, Peak: 40, Base: 10, PeakFrom: 9, PeakTo: 17, Windowed: true},
		{Name: "b", Provisioned: 100, Peak: 40, Base: 10, PeakFrom: 20, PeakTo: 23, Windowed: true},
	}, estate.Assumptions{Headroom: 20, Years: 1})
}

func TestSeriesFor(t *testing.T) {
	s := graphFixture()
	series := seriesFor(s)

	if len(series) != 3 {
		t.Fatalf("expected 3 series, got %d", len(series))
	}

	// Every sizing racks one capacity and holds it all day.
	for _, sr := range series {
		for hour, value := range sr.values {
			if value != sr.values[0] {
				t.Fatalf("%s: hour %d is %g, expected the flat %g", sr.label, hour, value, sr.values[0])
			}
		}
	}

	if got := series[0].values[0]; got != s.RackedToday.Required {
		t.Errorf("racked today: expected %g, got %g", s.RackedToday.Required, got)
	}

	if got := series[1].values[0]; got != s.SumOfPeaks.Required {
		t.Errorf("sum of the peaks: expected %g, got %g", s.SumOfPeaks.Required, got)
	}

	// The peak of the sum line is the capacity the table reports, held all
	// day, and it must sit below the sum of the peaks for peaks falling in
	// different windows to be worth anything.
	if got := series[2].values[0]; got != s.PeakOfSum.Required {
		t.Errorf("peak of the sum: expected %g, got %g", s.PeakOfSum.Required, got)
	}

	if series[2].peak() >= series[1].peak() {
		t.Errorf("peak of the sum: %g is not below the sum of the peaks %g", series[2].peak(), series[1].peak())
	}
}

// Without peak windows there is no peak of the sum sizing to plot, so the chart
// must not imply one.
func TestSeriesForWithoutWindows(t *testing.T) {
	s := estate.Summarise([]estate.Cluster{{Name: "a", Provisioned: 100, Peak: 40}}, estate.Assumptions{Headroom: 20, Years: 1})

	if got := len(seriesFor(s)); got != 2 {
		t.Errorf("expected 2 series, got %d", got)
	}
}

func TestGraphView(t *testing.T) {
	m := newModel(graphFixture())

	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := model.View()

	for _, want := range []string{"Utilisation", "Racked today", "Sum of the peaks", "Peak of the sum", quitPrompt, "00", "23"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected the view to contain %q", want)
		}
	}

	if !strings.Contains(view, full) {
		t.Error("expected the view to contain bars")
	}
}

// The chart must stay inside the terminal, however narrow, or it wraps and
// the bars stop lining up with their hours.
func TestGraphFitsTheTerminal(t *testing.T) {
	m := newModel(graphFixture())

	for _, width := range []int{20, 40, 60, 80, 96, 120, 200} {
		model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})

		for i, line := range strings.Split(model.View(), "\n") {
			if got := ansi.StringWidth(line); got > width {
				t.Errorf("width %d: line %d is %d columns wide", width, i, got)
			}
		}
	}
}

// A terminal too narrow for one column an hour gets a message rather than a
// chart whose lines have drifted off their hours.
func TestGraphTooNarrowToPlot(t *testing.T) {
	m := newModel(graphFixture())
	m.width = m.minWidth() - 1

	if got := m.chart(); !strings.Contains(got, "Needs") {
		t.Errorf("expected a message about the terminal being too narrow, got %q", got)
	}
}

// The chart grows with the terminal, within the bounds that keep it readable
// and leave room for the furniture around it.
func TestGraphChartHeight(t *testing.T) {
	m := newModel(graphFixture())

	cases := map[int]int{0: minChartHeight, 10: minChartHeight, 20: 13, 100: maxChartHeight}

	for height, exp := range cases {
		m.height = height
		if got := m.chartHeight(); got != exp {
			t.Errorf("height %d: expected %d chart rows, got %d", height, exp, got)
		}
	}
}

func TestGraphQuits(t *testing.T) {
	m := newModel(graphFixture())

	for _, key := range []string{"q", "esc", "ctrl+c"} {
		if _, cmd := m.Update(tea.KeyMsg(tea.Key{Type: keyType(key), Runes: []rune(key)})); cmd == nil {
			t.Errorf("%s: expected a quit command", key)
		}
	}

	// Anything else leaves the chart up.
	if _, cmd := m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune("x")})); cmd != nil {
		t.Error("expected an unrecognised key to be ignored")
	}

	if cmd := m.Init(); cmd != nil {
		t.Error("expected no work on startup")
	}
}

// An empty estate has nothing to scale the bars against, so the chart must say
// so rather than divide by zero.
func TestGraphWithNothingToPlot(t *testing.T) {
	m := newModel(estate.Summarise([]estate.Cluster{{Name: "a"}}, estate.Assumptions{Headroom: 20, Years: 1}))

	if got := m.chart(); got != emptyChart {
		t.Errorf("expected %q, got %q", emptyChart, got)
	}
}

// Every sizing is labelled on the axis at the row its peak reaches, along with
// the busiest hour of the utilisation curve, each in its own colour.
func TestGraphAxisLabels(t *testing.T) {
	m := newModel(graphFixture())

	const height = 16

	labels := m.axisLabels(height)

	exp := map[string]lipgloss.Color{
		"200": rackedTodayColours.label, // what is racked today
		"96":  sumOfPeaksColours.label,  // ceil(80 * 1.2)
		"60":  peakOfSumColours.label,   // ceil(50 * 1.2)
		"50":  utilisationColours.label, // the busiest hour's demand
		"0":   "",
	}

	got := map[string]lipgloss.Color{}

	for row, label := range labels {
		if row < 0 || row >= height {
			t.Errorf("label %q sits on row %d, outside the chart", label.text, row)
		}

		got[label.text] = label.colour
	}

	for text, colour := range exp {
		if got[text] != colour {
			t.Errorf("expected %q in colour %q, got %q", text, colour, got[text])
		}
	}

	if len(got) != len(exp) {
		t.Errorf("expected %d labels, got %v", len(exp), got)
	}
}

// A short chart has fewer rows than figures to label, and two peaks sharing a
// row can only show one number.
func TestGraphAxisLabelsShareRows(t *testing.T) {
	m := newModel(graphFixture())

	labels := m.axisLabels(minChartHeight)

	rows := slices.Sorted(maps.Keys(labels))
	for i := 1; i < len(rows); i++ {
		if rows[i] == rows[i-1] {
			t.Fatalf("two labels on row %d", rows[i])
		}
	}

	// The taller of two peaks in the same band keeps the number, so the hour
	// aware line's 60 displaces the 50 of demand beneath it.
	if text := labels[1].text; text != "60" {
		t.Errorf("expected the taller peak to take the row, got %q", text)
	}

	for row, label := range labels {
		if label.text == "50" {
			t.Errorf("expected the shorter peak to go unlabelled, found it on row %d", row)
		}
	}
}

// A line claims the row it lands on, whichever fills are stacked behind it,
// and everything under a line is its own translucent fill. That is what makes
// idle capacity readable at a glance: the gap between a sizing's line and the
// utilisation beneath it.
func TestGraphCell(t *testing.T) {
	m := newModel(graphFixture())

	// A 200 axis over 10 rows puts a row every 20 vcpu. Hour 9 draws 50 of
	// demand and hour 0 only 20, under the peak of the sum 60, the sum of the
	// peaks 96 and racked today 200.
	const height = 10

	cases := []struct {
		name   string
		hour   int
		row    int
		block  string
		colour lipgloss.Color
	}{
		{name: "demand fills the rows under its line", hour: 9, row: 0, block: shade, colour: utilisationColours.fill},

		// The peak of the sum 60 lands on the same row as the 50 of demand, and the
		// lower of the two takes it.
		{name: "the lower of two lines sharing a row", hour: 9, row: 2, block: full, colour: utilisationColours.fill},

		{name: "the lowest fill over the demand", hour: 0, row: 1, block: shade, colour: peakOfSumColours.fill},
		{name: "peak of the sum line", hour: 0, row: 2, block: full, colour: peakOfSumColours.fill},
		{name: "sum of the peaks line", hour: 0, row: 4, block: full, colour: sumOfPeaksColours.fill},
		{name: "racked today line at the top", hour: 0, row: height - 1, block: full, colour: rackedTodayColours.fill},
		{name: "nothing above the highest line", hour: 0, row: height, block: " ", colour: ""},
	}

	// One column an hour, so a column is an hour and the curve passes straight
	// through the readings.
	curves := m.curves(1, height)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			block, colour := m.cell(curves, c.hour, c.row)

			if block != c.block || colour != c.colour {
				t.Errorf("expected %q in %q, got %q in %q", c.block, c.colour, block, colour)
			}
		})
	}
}

// Every line takes a whole row, however little of it the value reaches into,
// so no line reads lighter than another. A partly filled cell leaves the rest
// of itself to the terminal's own background, which punches a notch through
// whatever the line is drawn over.
func TestGraphCellLinesAreWholeBlocks(t *testing.T) {
	m := newModel(graphFixture())

	const height = 10

	// A line at 1 of a 200 axis barely reaches into row 0, and one at 200
	// fills row 9 to the top. Both are drawn the same.
	for _, value := range []float64{1, 185, 200} {
		sizing := curve{colours: peakOfSumColours, columns: m.spans([]float64{value}, height)}

		row, _ := m.lineRow(value, height)

		if block, colour := m.cell([]curve{sizing}, 0, row); block != full || colour != peakOfSumColours.fill {
			t.Errorf("%g: expected %q in %q, got %q in %q", value, full, peakOfSumColours.fill, block, colour)
		}
	}
}

// The curve has to pass through the hours it was drawn from, and a sizing
// that holds one capacity all day has to come out of it flat.
func TestGraphSmooth(t *testing.T) {
	m := newModel(graphFixture())

	// An odd number of columns an hour puts one of them on the hour itself.
	const cell = 3

	curve := smooth(m.used, cell)

	if got := len(curve); got != estate.HoursPerDay*cell {
		t.Fatalf("expected %d columns, got %d", estate.HoursPerDay*cell, got)
	}

	for hour, exp := range m.used {
		if got := curve[hour*cell+cell/2]; math.Abs(got-exp) > 1e-9 {
			t.Errorf("hour %d: expected the curve through %g, got %g", hour, exp, got)
		}
	}

	flat := smooth(m.series[0].values, cell)
	for col, got := range flat {
		if got != flat[0] {
			t.Fatalf("column %d of a flat sizing is %g, expected %g", col, got, flat[0])
		}
	}
}

// A plain spline rounding a peak carries the curve above the busiest hour it
// was drawn from, which would have the chart claim demand the estate never
// sees. The monotone one has to stay inside its readings.
func TestGraphSmoothDoesNotOvershoot(t *testing.T) {
	m := newModel(graphFixture())

	lowest, highest := slices.Min(m.used[:]), slices.Max(m.used[:])

	for col, got := range smooth(m.used, maxCellWidth) {
		if got < lowest || got > highest {
			t.Errorf("column %d is %g, outside the %g to %g the hours gave", col, got, lowest, highest)
		}
	}
}

// A line that climbs a long way between one column and the next has to stay
// joined up, or it draws as a dotted trail rather than a line.
func TestGraphSpansJoinUp(t *testing.T) {
	m := newModel(graphFixture())

	const height = 10

	// The axis tops out at 200, so these are rows 0 and 9 of 10.
	spans := m.spans([]float64{1, 200, 1}, height)

	for col, s := range spans[:len(spans)-1] {
		next := spans[col+1]

		if s.hi+1 < next.lo || next.hi+1 < s.lo {
			t.Errorf("columns %d and %d leave a gap between rows %d-%d and %d-%d", col, col+1, s.lo, s.hi, next.lo, next.hi)
		}
	}

	// A level line has nothing to reach for, so it stays on its own row.
	if level := m.spans([]float64{185, 185}, height); level[0].lo != 9 || level[0].hi != 9 {
		t.Errorf("expected a level line to stay on row 9, got rows %d-%d", level[0].lo, level[0].hi)
	}
}

// A line lands on the topmost row it reaches into, however far into it that
// is, and an hour of no demand has no line to draw at all.
func TestGraphLineRow(t *testing.T) {
	m := newModel(graphFixture())

	const height = 10

	cases := []struct {
		value float64
		exp   int
		ok    bool
	}{
		{value: 0, ok: false},
		{value: 200, exp: 9, ok: true},
		{value: 185, exp: 9, ok: true},
		{value: 20, exp: 0, ok: true},

		// A line barely into its row still takes the whole of it.
		{value: 1, exp: 0, ok: true},

		// Nothing is plotted off the top of the chart.
		{value: 400, exp: 9, ok: true},
	}

	for _, c := range cases {
		got, ok := m.lineRow(c.value, height)

		if ok != c.ok || got != c.exp {
			t.Errorf("%g: expected row %d (%v), got %d (%v)", c.value, c.exp, c.ok, got, ok)
		}
	}
}

// The legend keys the shared blue first, then one entry per sizing, and wraps
// onto its own lines when the terminal can't hold it.
func TestGraphLegend(t *testing.T) {
	m := newModel(graphFixture())

	if got := len(m.keys()); got != len(m.series)+1 {
		t.Fatalf("expected an entry per sizing plus the utilisation key, got %d", got)
	}

	if got := m.keys()[0].label; got != utilisationLabel {
		t.Errorf("expected %q first, got %q", utilisationLabel, got)
	}

	m.width = 200
	if got := strings.Count(m.legend(), "\n"); got != 0 {
		t.Errorf("expected the legend on one line, got %d line breaks", got)
	}

	m.width = 30
	if got := strings.Count(m.legend(), "\n"); got != len(m.keys())-1 {
		t.Errorf("expected one legend entry per line, got %d line breaks", got)
	}
}

// Narrow terminals can't label every hour without the numbers colliding, so
// the axis thins out instead.
func TestGraphHourAxis(t *testing.T) {
	m := newModel(graphFixture())

	wide := m.hourAxis(4)
	if !strings.Contains(wide, "22") {
		t.Errorf("expected every hour to be labelled, got %q", wide)
	}

	// One column an hour only leaves room for every third label.
	thinned := m.hourAxis(1)
	for hour := range estate.HoursPerDay {
		labelled := strings.Contains(thinned, fmt.Sprintf("%02d", hour))

		if exp := hour%3 == 0; labelled != exp {
			t.Errorf("hour %d: expected labelled %v, got %q", hour, exp, thinned)
		}
	}
}

func keyType(key string) tea.KeyType {
	switch key {
	case "esc":
		return tea.KeyEsc
	case "ctrl+c":
		return tea.KeyCtrlC
	default:
		return tea.KeyRunes
	}
}
