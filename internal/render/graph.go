package render

import (
	"fmt"
	"io"
	"math"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/codingconcepts/u9n/internal/estate"
)

const (
	// barWidth is the width in columns of a single sizing's bar, and gutter the
	// gap between one hour's group of bars and the next.
	barWidth = 1
	gutter   = 1

	minChartHeight = 6
	maxChartHeight = 20

	// chartPadding is the rows the chart gives up to the header, hour axis and
	// prompt around it.
	chartPadding = 8

	// full and half are the blocks the bars are drawn from. A half block lets a
	// bar land between two rows, doubling the vertical resolution.
	full = "█"
	half = "▄"

	// legendGap separates the legend's entries when they share a line.
	legendGap = "   "

	utilisationLabel = "Utilisation"
	quitPrompt       = "q to quit"
	emptyChart       = "No vcpu to plot."
)

// palette is the pair of colours one line is drawn in. The bar takes the fill,
// the brand colour at full strength, and the number on the axis the lighter
// tint of it, which carries the same hue but reads as text against the
// background.
type palette struct {
	label lipgloss.Color
	fill  lipgloss.Color
}

// The fills are three brand hues far enough apart that no two bars standing
// next to each other read as the same colour, under colour blind vision
// included, and a neutral for the demand underneath them. The demand is drawn
// in one colour because it is a single curve shared by all three sizings
// rather than a shade per line, and in the taupe a step down from the brand's
// lightest because a near white block that size glares on a dark terminal.
var (
	currentColours     = palette{label: "#AFF6FF", fill: "#60ECFF"} // Sci-fi Blue
	worstCaseColours   = palette{label: "#D7EEA9", fill: "#AEDE54"} // Bio Green
	hourAwareColours   = palette{label: "#C8ACFF", fill: "#9059FF"} // Data Purple
	utilisationColours = palette{label: "#E5E4DC", fill: "#A0A09A"} // Timeless Taupe
)

// series is one sizing's capacity across the 24 hour clock.
type series struct {
	label   string
	colours palette
	values  [estate.HoursPerDay]float64
}

// peak is the tallest hour of a line, which is the whole line, since every
// sizing holds one capacity all day.
func (s series) peak() float64 {
	return slices.Max(s.values[:])
}

// Graph opens a terminal chart of what each sizing provisions across the day,
// drawing to w so the caller decides where the output lands.
func Graph(w io.Writer, s estate.Summary) error {
	if _, err := tea.NewProgram(newModel(s), tea.WithAltScreen(), tea.WithOutput(w)).Run(); err != nil {
		return fmt.Errorf("running graph: %w", err)
	}

	return nil
}

type model struct {
	summary estate.Summary
	series  []series

	// used is the demand the estate has to serve each hour, and axisTop the
	// capacity the tallest bar reaches. Demand is one curve for the whole
	// chart, because the workload doesn't change with the way it is sized.
	used    [estate.HoursPerDay]float64
	axisTop float64

	width  int
	height int
}

func newModel(s estate.Summary) model {
	m := model{
		summary: s,
		series:  seriesFor(s),

		// Sensible defaults for the first frame, before the terminal has told
		// us how big it is.
		width:  96,
		height: 24,
	}

	for _, sr := range m.series {
		m.axisTop = max(m.axisTop, sr.peak())
	}

	for hour, total := range s.Hourly {
		m.used[hour] = total * s.GrowthFactor
	}

	return m
}

// seriesFor turns each sizing into an hourly capacity line. Every sizing racks
// one capacity and holds it all day, the hour aware one included: it is sized
// by the estate's busiest hour rather than by every cluster's peak at once, but
// the hardware still sits there the other 23 hours. The gap between a line and
// the demand beneath it is the capacity that sizing leaves idle.
func seriesFor(s estate.Summary) []series {
	line := func(sc estate.Scenario, colours palette) series {
		sr := series{label: sc.Label, colours: colours}
		for hour := range sr.values {
			sr.values[hour] = sc.Required
		}

		return sr
	}

	out := []series{
		line(s.Current, currentColours),
		line(s.WorstCase, worstCaseColours),
	}

	// Without peak windows the hour aware sizing is the worst case, so plotting
	// it would draw the same line twice.
	if !s.Windowed {
		return out
	}

	return append(out, line(s.HourAware, hourAwareColours))
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) View() string {
	var b strings.Builder

	b.WriteString(m.header())
	b.WriteString("\n\n")
	b.WriteString(m.chart())
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render(quitPrompt))
	b.WriteString("\n")

	return b.String()
}

// legendKey is one entry of the legend: a swatch and what it stands for.
type legendKey struct {
	label  string
	colour lipgloss.Color
}

// keys lists the legend in the order the eye meets it in a bar, the shared
// utilisation blue first and then the sizings stacked on top of it.
func (m model) keys() []legendKey {
	keys := []legendKey{{label: utilisationLabel, colour: utilisationColours.fill}}
	for _, sr := range m.series {
		keys = append(keys, legendKey{label: sr.label, colour: sr.colours.fill})
	}

	return keys
}

func (m model) header() string {
	title := fmt.Sprintf("vCPU provisioned across the day, %d clusters, %s%% headroom",
		len(m.summary.Clusters), estate.Num(m.summary.Headroom))

	var (
		legend []string
		width  int
	)

	for _, k := range m.keys() {
		swatch := lipgloss.NewStyle().Foreground(k.colour).Render(full + full)
		entry := ansi.Truncate(fmt.Sprintf("%s %s", swatch, k.label), m.width, "")

		legend = append(legend, entry)
		width += ansi.StringWidth(entry) + len(legendGap)
	}

	// The legend goes on one line if it fits, and one entry per line if not.
	separator := legendGap
	if width > m.width {
		separator = "\n"
	}

	return lipgloss.NewStyle().Bold(true).Render(ansi.Truncate(title, m.width, "")) +
		"\n" + strings.Join(legend, separator)
}

// minWidth is the narrowest the chart can be drawn without bars overlapping,
// which is one column per sizing per hour plus the axis labels.
func (m model) minWidth() int {
	return m.labelWidth() + estate.HoursPerDay*len(m.series)*barWidth
}

// chartHeight leaves room for the header, axis, legend and prompt.
func (m model) chartHeight() int {
	return min(max(m.height-chartPadding, minChartHeight), maxChartHeight)
}

// groupWidth is the columns one hour occupies, shrinking the gutter and then
// the bars themselves before giving up on a narrow terminal.
func (m model) groupWidth() int {
	bars := len(m.series) * barWidth

	if m.labelWidth()+estate.HoursPerDay*(bars+gutter) <= m.width {
		return bars + gutter
	}

	return bars
}

// labelWidth is the gutter the y axis numbers are right aligned in, sized by
// the largest of them.
func (m model) labelWidth() int {
	return len(estate.Num(m.axisTop)) + 1
}

func (m model) chart() string {
	if m.axisTop == 0 {
		return emptyChart
	}

	if m.width < m.minWidth() {
		return ansi.Truncate(fmt.Sprintf("Terminal is %d columns, the chart needs %d.", m.width, m.minWidth()), m.width, "")
	}

	height := m.chartHeight()
	group := m.groupWidth()
	labels := m.axisLabels(height)

	var b strings.Builder

	for row := height - 1; row >= 0; row-- {
		// Each row spans a band of vcpu. A bar reaching the top of the band
		// gets a full block, one reaching halfway gets a half block.
		band := m.axisTop / float64(height)
		bandTop := band * float64(row+1)
		bandMid := band * (float64(row) + 0.5)

		b.WriteString(labels[row].render(m.labelWidth()))

		for hour := range estate.HoursPerDay {
			for _, sr := range m.series {
				block := " "

				switch value := sr.values[hour]; {
				case value >= bandTop:
					block = full
				case value >= bandMid:
					block = half
				}

				b.WriteString(lipgloss.NewStyle().
					Foreground(m.barColour(sr, hour, bandMid)).
					Render(strings.Repeat(block, barWidth)))
			}

			b.WriteString(strings.Repeat(" ", group-len(m.series)*barWidth))
		}

		b.WriteString("\n")
	}

	b.WriteString(m.hourAxis(group))

	return b.String()
}

// barColour is the colour a block takes: the shared blue while the bar is
// still inside the demand, and the sizing's own fill for the capacity
// sitting idle above it.
func (m model) barColour(sr series, hour int, bandMid float64) lipgloss.Color {
	if m.used[hour] >= bandMid {
		return utilisationColours.fill
	}

	return sr.colours.fill
}

// axisLabel is one number printed against the y axis, coloured to match what
// it belongs to.
type axisLabel struct {
	text   string
	colour lipgloss.Color
}

// render right aligns the label in the axis gutter, which is one column wider
// than the longest number so the bars don't touch it.
func (a axisLabel) render(width int) string {
	text := a.text
	if a.colour != "" {
		text = lipgloss.NewStyle().Foreground(a.colour).Render(text)
	}

	return strings.Repeat(" ", max(width-1-len(a.text), 0)) + text + " "
}

// axisLabels pairs rows with the numbers printed against them. Every line gets
// its peak labelled on the row its tallest bar reaches, plus the busiest hour
// of the utilisation curve, so each figure can be read off the axis rather
// than guessed at from the bars.
func (m model) axisLabels(height int) map[int]axisLabel {
	labels := map[int]axisLabel{0: {text: "0"}}
	peaks := map[int]float64{}

	mark := func(value float64, colour lipgloss.Color) {
		if value == 0 {
			return
		}

		row := min(max(int(math.Ceil(value/m.axisTop*float64(height)))-1, 0), height-1)

		// Two peaks in the same band have one row between them, so the taller
		// of the two takes the label.
		if existing, ok := peaks[row]; ok && existing >= value {
			return
		}

		peaks[row] = value
		labels[row] = axisLabel{text: estate.Num(value), colour: colour}
	}

	for _, sr := range m.series {
		mark(sr.peak(), sr.colours.label)
	}

	mark(slices.Max(m.used[:]), utilisationColours.label)

	return labels
}

// hourAxis labels as many hours as fit without the labels running into each
// other.
func (m model) hourAxis(group int) string {
	// max guards the loop: a group of no columns would never reach the
	// spacing an hour label needs.
	every := 1
	for max(group, 1)*every < 3 {
		every++
	}

	var b strings.Builder
	b.WriteString(strings.Repeat(" ", m.labelWidth()))

	for hour := 0; hour < estate.HoursPerDay; hour += every {
		text := fmt.Sprintf("%02d", hour)
		b.WriteString(text)
		b.WriteString(strings.Repeat(" ", max(group*every-len(text), 0)))
	}

	return strings.TrimRight(b.String(), " ")
}
