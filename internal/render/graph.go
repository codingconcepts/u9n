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
	// maxCellWidth is the widest one hour is drawn, so a roomy terminal
	// stretches the lines rather than stranding the chart in a corner.
	maxCellWidth = 5

	minChartHeight = 6
	maxChartHeight = 20

	// chartPadding is the rows the chart gives up to the legend, hour axis and
	// prompt around it.
	chartPadding = 7

	// full is the block a line is drawn from, and shade the translucent fill
	// beneath it, light enough that the lines crossing it and the fills behind
	// it still read.
	full  = "█"
	shade = "░"

	// legendGap separates the legend's entries when they share a line.
	legendGap = "   "

	utilisationLabel = "Utilisation"
	quitPrompt       = "q to quit"
	emptyChart       = "No vcpu to plot."
)

// palette is the pair of colours one line is drawn in. The line and the fill
// beneath it take the fill colour, the brand colour at full strength, and the
// number on the axis the lighter tint of it, which carries the same hue but
// reads as text against the background.
type palette struct {
	label lipgloss.Color
	fill  lipgloss.Color
}

// The fills are three brand hues far enough apart that no two lines crossing
// each other read as the same colour, under colour blind vision included, and
// a neutral for the demand underneath them. The demand is drawn in one colour
// because it is a single curve shared by all three sizings rather than a shade
// per line, and in the taupe a step down from the brand's lightest because a
// near white fill that size glares on a dark terminal.
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
	series []series

	// used is the demand the estate has to serve each hour, and axisTop the
	// capacity the highest line reaches. Demand is one curve for the whole
	// chart, because the workload doesn't change with the way it is sized.
	used    [estate.HoursPerDay]float64
	axisTop float64

	width  int
	height int
}

func newModel(s estate.Summary) model {
	m := model{
		series: seriesFor(s),

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

	b.WriteString(m.legend())
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

// keys lists the legend from the bottom of the chart up, the shared
// utilisation curve first and then the sizings drawn over it.
func (m model) keys() []legendKey {
	keys := []legendKey{{label: utilisationLabel, colour: utilisationColours.fill}}
	for _, sr := range m.series {
		keys = append(keys, legendKey{label: sr.label, colour: sr.colours.fill})
	}

	return keys
}

// legend keys what the chart draws, one swatch an entry.
func (m model) legend() string {
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

	return strings.Join(legend, separator)
}

// minWidth is the narrowest the chart can be drawn without hours sharing a
// column, which is one column an hour plus the axis labels.
func (m model) minWidth() int {
	return m.labelWidth() + estate.HoursPerDay
}

// chartHeight leaves room for the axis, legend and prompt.
func (m model) chartHeight() int {
	return min(max(m.height-chartPadding, minChartHeight), maxChartHeight)
}

// cellWidth is the columns one hour occupies, widening with the terminal up to
// the point where the lines are drawn thicker than they are long.
func (m model) cellWidth() int {
	return min(max((m.width-m.labelWidth())/estate.HoursPerDay, 1), maxCellWidth)
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
		// The chart now fits in not much more than the 24 columns of the clock,
		// so the message saying it doesn't has to fit in fewer still.
		return ansi.Truncate(fmt.Sprintf("Needs %d columns, got %d.", m.minWidth(), m.width), m.width, "")
	}

	height := m.chartHeight()
	cell := m.cellWidth()
	labels := m.axisLabels(height)
	curves := m.curves(cell, height)

	var b strings.Builder

	for row := height - 1; row >= 0; row-- {
		b.WriteString(labels[row].render(m.labelWidth()))

		for col := range estate.HoursPerDay * cell {
			block, colour := m.cell(curves, col, row)

			if colour == "" {
				b.WriteString(block)
				continue
			}

			b.WriteString(lipgloss.NewStyle().Foreground(colour).Render(block))
		}

		b.WriteString("\n")
	}

	b.WriteString(m.hourAxis(cell))

	return b.String()
}

// layers is everything the chart draws, back to front: the demand curve
// underneath, and one line per sizing over it. Where two of them cross, the
// lower one wins the cell, so a line is never buried by the fill of something
// taller.
func (m model) layers() []series {
	return append([]series{{
		label:   utilisationLabel,
		colours: utilisationColours,
		values:  m.used,
	}}, m.series...)
}

// curve is a layer resampled to the columns it's drawn in, one span a column
// rather than one value an hour.
type curve struct {
	colours palette
	columns []span
}

// span is the rows one column of a line covers. A line is more than the row
// its value lands on: it stretches half way to the row either neighbour lands
// on, so a steep climb draws as a joined-up line rather than a dotted one.
type span struct {
	value  float64
	lo, hi int

	// drawn is false for a column with no demand, which has no line at all.
	drawn bool
}

// curves rounds every layer off into the line it's drawn as. A flat sizing
// stays flat, so in practice it's the demand underneath that bends.
func (m model) curves(cell, height int) []curve {
	layers := m.layers()
	out := make([]curve, 0, len(layers))

	for _, sr := range layers {
		out = append(out, curve{colours: sr.colours, columns: m.spans(smooth(sr.values, cell), height)})
	}

	return out
}

// spans turns a column's worth of values into the rows each is drawn across,
// joining every column to the two beside it. Both columns either side of a
// gap round the row between them the same way, so the halves they each draw
// meet rather than leaving the line broken.
func (m model) spans(values []float64, height int) []span {
	var (
		out  = make([]span, len(values))
		rows = make([]int, len(values))
	)

	for col, value := range values {
		row, ok := m.lineRow(value, height)
		if !ok {
			continue
		}

		rows[col] = row
		out[col] = span{value: value, lo: row, hi: row, drawn: true}
	}

	// The rows a column is stretched to are taken from where its neighbours
	// landed, not from how far they have themselves been stretched, or one
	// steep column would drag the whole line after it.
	for col, s := range out {
		if !s.drawn {
			continue
		}

		for _, neighbour := range []int{col - 1, col + 1} {
			if neighbour < 0 || neighbour >= len(out) || !out[neighbour].drawn {
				continue
			}

			mid := (s.lo + rows[neighbour]) / 2
			out[col].lo = min(out[col].lo, mid)
			out[col].hi = max(out[col].hi, mid)
		}
	}

	return out
}

// smooth resamples an hourly series to one value a column, running a monotone
// cubic through the 24 readings so a line curves from one hour into the next
// instead of stepping. Monotone rather than a plain spline because a spline
// overshoots, and a demand curve bulging above the busiest hour it was drawn
// from is a lie about the workload.
func smooth(values [estate.HoursPerDay]float64, cell int) []float64 {
	var secants, tangents [estate.HoursPerDay]float64

	// The day is a loop, so hour 23 runs back into hour 0 and the curve
	// doesn't flatten off against either edge of the chart.
	for i := range secants {
		secants[i] = values[(i+1)%estate.HoursPerDay] - values[i]
	}

	for i := range tangents {
		// A turning point or a flat run keeps its tangent at zero, which is
		// what stops the curve rounding over the hour it passes through.
		if prev := secants[(i+estate.HoursPerDay-1)%estate.HoursPerDay]; prev*secants[i] > 0 {
			tangents[i] = (prev + secants[i]) / 2
		}
	}

	// Fritsch-Carlson: pull in any tangent steep enough to carry the curve
	// past one of the two readings it joins.
	for i, d := range secants {
		if d == 0 {
			continue
		}

		next := (i + 1) % estate.HoursPerDay
		alpha, beta := tangents[i]/d, tangents[next]/d

		if sum := alpha*alpha + beta*beta; sum > 9 {
			tau := 3 / math.Sqrt(sum)
			tangents[i], tangents[next] = tau*alpha*d, tau*beta*d
		}
	}

	out := make([]float64, estate.HoursPerDay*cell)

	for col := range out {
		// An hour's reading sits at the middle of its block of columns, so the
		// curve passes through the hour rather than starting at it.
		at := (float64(col)+0.5)/float64(cell) - 0.5

		hour := int(math.Floor(at))
		t := at - float64(hour)
		hour = ((hour % estate.HoursPerDay) + estate.HoursPerDay) % estate.HoursPerDay
		next := (hour + 1) % estate.HoursPerDay

		// The Hermite basis, which is the curve itself: the two readings
		// either side of the column, weighted against the slope at each.
		square, cube := t*t, t*t*t

		value := (2*cube-3*square+1)*values[hour] +
			(cube-2*square+t)*tangents[hour] +
			(3*square-2*cube)*values[next] +
			(cube-square)*tangents[next]

		// The curve is already held between the two hours it joins, so this
		// only takes off the rounding the basis leaves behind. Without it a
		// flat sizing comes out a hair off flat.
		out[col] = min(max(value, min(values[hour], values[next])), max(values[hour], values[next]))
	}

	return out
}

// cell is the block one column of one row is drawn with, and the colour it
// takes. A line claims the row it lands on, and the rows beneath it are the
// fill, drawn in a light shade so the lines behind still read through it.
func (m model) cell(curves []curve, col, row int) (string, lipgloss.Color) {
	var (
		lineColour, fillColour lipgloss.Color

		// The frontmost line and fill found so far, which are the lowest of
		// them, since a line can only be hidden by one drawn under it.
		line = math.Inf(1)
		area = math.Inf(1)
	)

	for _, sr := range curves {
		s := sr.columns[col]
		if !s.drawn {
			continue
		}

		switch {
		case row >= s.lo && row <= s.hi && s.value < line:
			line, lineColour = s.value, sr.colours.fill

		case row < s.lo && s.value < area:
			area, fillColour = s.value, sr.colours.fill
		}
	}

	switch {
	case !math.IsInf(line, 1):
		return full, lineColour

	case !math.IsInf(area, 1):
		return shade, fillColour
	}

	return " ", ""
}

// lineRow is the row a value's line lands on, which is the topmost row it
// reaches into, however far into it that is. Nothing is drawn for a column
// with no demand.
func (m model) lineRow(value float64, height int) (int, bool) {
	if value <= 0 {
		return 0, false
	}

	// The rows the value fills, fractional, so the row it reaches into is the
	// one the count rounds up to.
	rows := value / m.axisTop * float64(height)

	return min(max(int(math.Ceil(rows))-1, 0), height-1), true
}

// axisLabel is one number printed against the y axis, coloured to match what
// it belongs to.
type axisLabel struct {
	text   string
	colour lipgloss.Color
}

// render right aligns the label in the axis gutter, which is one column wider
// than the longest number so the chart doesn't touch it.
func (a axisLabel) render(width int) string {
	text := a.text
	if a.colour != "" {
		text = lipgloss.NewStyle().Foreground(a.colour).Render(text)
	}

	return strings.Repeat(" ", max(width-1-len(a.text), 0)) + text + " "
}

// axisLabels pairs rows with the numbers printed against them. Every line gets
// its peak labelled on the row it reaches, plus the busiest hour of the
// utilisation curve, so each figure can be read off the axis rather than
// guessed at from the chart.
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
func (m model) hourAxis(cell int) string {
	// max guards the loop: a cell of no columns would never reach the spacing
	// an hour label needs.
	every := 1
	for max(cell, 1)*every < 3 {
		every++
	}

	var b strings.Builder
	b.WriteString(strings.Repeat(" ", m.labelWidth()))

	for hour := 0; hour < estate.HoursPerDay; hour += every {
		text := fmt.Sprintf("%02d", hour)
		b.WriteString(text)
		b.WriteString(strings.Repeat(" ", max(cell*every-len(text), 0)))
	}

	return strings.TrimRight(b.String(), " ")
}
