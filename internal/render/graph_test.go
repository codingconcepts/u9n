package render

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/codingconcepts/u9n/internal/estate"
)

// graphFixture is two clusters peaking in different windows, so the hour aware
// line moves and the three sizings differ.
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

	if got := series[0].values[0]; got != s.Current.Required {
		t.Errorf("current: expected %g, got %g", s.Current.Required, got)
	}

	if got := series[1].values[0]; got != s.WorstCase.Required {
		t.Errorf("worst case: expected %g, got %g", s.WorstCase.Required, got)
	}

	// The hour aware line is the capacity the table reports, held all day, and
	// it must sit below the worst case for peaks falling in different windows
	// to be worth anything.
	if got := series[2].values[0]; got != s.HourAware.Required {
		t.Errorf("hour aware: expected %g, got %g", s.HourAware.Required, got)
	}

	if series[2].peak() >= series[1].peak() {
		t.Errorf("hour aware: %g is not below the worst case %g", series[2].peak(), series[1].peak())
	}
}

// Without peak windows there is no hour aware sizing to plot, so the chart
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

	for _, want := range []string{"Utilisation", "Current", "Worst case", "Hour aware", quitPrompt, "00", "23"} {
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

// A terminal too narrow for one column per sizing per hour gets a message
// rather than a chart whose bars have drifted off their hours.
func TestGraphTooNarrowToPlot(t *testing.T) {
	m := newModel(graphFixture())
	m.width = m.minWidth() - 1

	if got := m.chart(); !strings.Contains(got, "the chart needs") {
		t.Errorf("expected a message about the terminal being too narrow, got %q", got)
	}
}

// The chart grows with the terminal, within the bounds that keep it readable
// and leave room for the furniture around it.
func TestGraphChartHeight(t *testing.T) {
	m := newModel(graphFixture())

	cases := map[int]int{0: minChartHeight, 10: minChartHeight, 20: 12, 100: maxChartHeight}

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
		"200": currentColours.label,     // what is racked today
		"96":  worstCaseColours.label,   // ceil(80 * 1.2)
		"60":  hourAwareColours.label,   // ceil(50 * 1.2)
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

// The bar is blue while it is inside the demand and the sizing's own fill
// above it, which is what makes idle capacity readable at a glance.
func TestGraphBarColour(t *testing.T) {
	m := newModel(graphFixture())
	sr := m.series[0]

	// Hour 9 draws 50 of demand, hour 0 only 20.
	cases := []struct {
		hour    int
		bandMid float64
		exp     lipgloss.Color
	}{
		{hour: 9, bandMid: 10, exp: utilisationColours.fill},
		{hour: 9, bandMid: 50, exp: utilisationColours.fill},
		{hour: 9, bandMid: 60, exp: sr.colours.fill},
		{hour: 0, bandMid: 30, exp: sr.colours.fill},
	}

	for _, c := range cases {
		if got := m.barColour(sr, c.hour, c.bandMid); got != c.exp {
			t.Errorf("hour %d at %g: expected %q, got %q", c.hour, c.bandMid, c.exp, got)
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
	if got := strings.Count(m.header(), "\n"); got != 1 {
		t.Errorf("expected the legend on one line, got %d lines after the title", got)
	}

	m.width = 30
	if got := strings.Count(m.header(), "\n"); got != len(m.keys()) {
		t.Errorf("expected one legend entry per line, got %d lines after the title", got)
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
