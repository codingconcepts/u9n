package render

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/codingconcepts/u9n/internal/estate"
)

// profileFixture is a single cluster peaking for two hours, so the chart has a
// base in all 24 hours and an uplift in two of them.
var profileFixture = []estate.Cluster{
	{Name: "a", Provisioned: 10, Peak: 5, Base: 1, PeakFrom: 9, PeakTo: 11, Windowed: true},
}

func TestProfileTable(t *testing.T) {
	var b strings.Builder
	if err := ProfileTable(&b, profileFixture, Bands{Base: true, Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	heading, rows, found := strings.Cut(b.String(), profileHeading+"\n\n")
	if !found || heading != "" {
		t.Fatalf("expected the table to open with %q, got:\n%s", profileHeading, b.String())
	}

	lines := strings.Split(strings.TrimRight(rows, "\n"), "\n")
	if len(lines) != estate.HoursPerDay {
		t.Fatalf("expected %d hours, got %d", estate.HoursPerDay, len(lines))
	}

	// Every hour tied for the busiest is marked, and the bars are drawn against
	// them, so an off peak hour gets a shorter one.
	for _, hour := range []int{9, 10} {
		exp := []string{estate.FormatHour(hour), strings.Repeat(profileBlock, profileTableWidth), "5", "<-", "peak"}
		if !slices.Equal(strings.Fields(lines[hour]), exp) {
			t.Errorf("expected peak hour row %q, got %q", exp, lines[hour])
		}
	}

	if exp := []string{"00:00", strings.Repeat(profileBlock, profileTableWidth/5), "1"}; !slices.Equal(strings.Fields(lines[0]), exp) {
		t.Errorf("expected off peak row %q, got %q", exp, lines[0])
	}
}

// Dropping a band rescales the rows the same way it rescales the chart, so the
// uplift on its own fills the widest bar rather than the four fifths of it the
// uplift sat in above the base.
func TestProfileTableDrawsOnlyTheBandsAskedFor(t *testing.T) {
	var b strings.Builder
	if err := ProfileTable(&b, profileFixture, Bands{Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")

	// Two lines of heading come first, so the peak hour is three lines further
	// down than its hour of the day.
	exp := []string{"09:00", strings.Repeat(profileBlock, profileTableWidth), "4", "<-", "peak"}
	if got := strings.Fields(lines[2+9]); !slices.Equal(got, exp) {
		t.Errorf("expected peak hour row %q, got %q", exp, got)
	}

	if exp := []string{"00:00", "0"}; !slices.Equal(strings.Fields(lines[2]), exp) {
		t.Errorf("expected an empty off peak row %q, got %q", exp, lines[2])
	}
}

func TestProfileTableRejectsATableWithNoBands(t *testing.T) {
	var b strings.Builder

	err := ProfileTable(&b, profileFixture, Bands{})
	if !errors.Is(err, errNoBands) {
		t.Fatalf("expected the empty table to be refused, got %v", err)
	}

	if b.String() != "" {
		t.Errorf("expected nothing to be written, got:\n%s", b.String())
	}
}

// An estate that draws nothing has no busiest hour to scale the rows against,
// so it says so rather than dividing by zero.
func TestProfileTableWithNoDemand(t *testing.T) {
	var b strings.Builder

	idle := []estate.Cluster{{Name: "a", Provisioned: 10}}
	if err := ProfileTable(&b, idle, Bands{Base: true, Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(b.String(), emptyProfile) {
		t.Errorf("expected %q, got:\n%s", emptyProfile, b.String())
	}

	if strings.Contains(b.String(), profileBlock) {
		t.Errorf("expected no bars for an idle estate, got:\n%s", b.String())
	}
}

func TestProfileTableReportsWriteErrors(t *testing.T) {
	err := ProfileTable(failingWriter{}, profileFixture, Bands{Base: true, Peak: true})
	if !errors.Is(err, errWrite) {
		t.Fatalf("expected the write error to surface, got %v", err)
	}
}

func TestProfileGraph(t *testing.T) {
	var b strings.Builder
	if err := ProfileGraph(&b, profileFixture, Bands{Base: true, Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The uplift reaches the top of the chart in the two peak hours, which are
	// marked as the busiest, and the base covers the foot of all 24.
	exp := strings.Join([]string{
		"",
		"░░ Base   ██ Peak   ▼ Busiest",
		"",
		"                    ▼ ▼",
		"5                   ████",
		"                    ████",
		"                    ████",
		"                    ████",
		"                    ████",
		"                    ████",
		"                    ████",
		"                    ████",
		"                    ████",
		"                    ████",
		"                    ████",
		"                    ████",
		"1 " + strings.Repeat("░", 48),
		"  " + strings.Repeat("░", 48),
		"  " + strings.Repeat("░", 48),
		"  " + strings.Repeat("░", 48),
		"0 " + strings.Repeat("─", 48),
		"  00    03    06    09    12    15    18    21",
		"",
		"",
	}, "\n")

	if b.String() != exp {
		t.Errorf("expected:\n%s\ngot:\n%s", exp, b.String())
	}
}

// Dropping a band rescales the chart, so the uplift on its own fills the
// height rather than staying where it sat above the base.
func TestProfileGraphDrawsOnlyTheBandsAskedFor(t *testing.T) {
	cases := []struct {
		name   string
		bands  Bands
		expTop string
		expLegend,
		expMissing string
	}{
		{
			name:       "base only",
			bands:      Bands{Base: true},
			expTop:     "1 ",
			expLegend:  baseLabel,
			expMissing: upliftLabel,
		},
		{
			name:       "peak only",
			bands:      Bands{Peak: true},
			expTop:     "4 ",
			expLegend:  upliftLabel,
			expMissing: baseLabel,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			if err := ProfileGraph(&b, profileFixture, c.bands); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			lines := strings.Split(b.String(), "\n")

			// The key comes first, with a blank line either side of it, and the
			// markers sit between it and the bars wherever the day has a
			// busiest hour to point at. The line under them is the tallest row
			// of the chart, which carries the number the axis is scaled to.
			top := lines[3]
			if strings.Contains(top, profileMarker) {
				top = lines[4]
			}

			if !strings.HasPrefix(top, c.expTop) {
				t.Errorf("expected the axis to top out at %q, got %q", c.expTop, top)
			}

			if !strings.Contains(b.String(), c.expLegend) {
				t.Errorf("expected the legend to key %q, got:\n%s", c.expLegend, b.String())
			}

			if strings.Contains(b.String(), c.expMissing) {
				t.Errorf("expected no %q band, got:\n%s", c.expMissing, b.String())
			}
		})
	}
}

// An hour a whisker below the peak rounds into the top row, which would draw
// it level with the hour it is below. The top row is kept for the busiest
// hours, so the two are a row apart and only the busiest is marked.
func TestProfileGraphKeepsTheTopRowForTheBusiestHours(t *testing.T) {
	// Two clusters whose windows overlap for an hour, with the second a
	// hundredth of the first, so the hour they share is barely above the eight
	// the first peaks in alone.
	clusters := []estate.Cluster{
		{Name: "a", Peak: 1000, PeakFrom: 9, PeakTo: 17, Windowed: true},
		{Name: "b", Peak: 10, PeakFrom: 16, PeakTo: 17, Windowed: true},
	}

	var b strings.Builder
	if err := ProfileGraph(&b, clusters, Bands{Base: true, Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(b.String(), "\n")

	// The markers come between the key and the bars, and the top row of the
	// chart is the line under them.
	markers, top := lines[3], lines[4]

	// One marker, over the hour the two clusters share, which is the only hour
	// to reach the top row.
	if exp := strings.Repeat(" ", len(estate.Num(1010))+1+16*profileCell) + profileMarker; markers != exp {
		t.Errorf("expected the marker at 16:00 %q, got %q", exp, markers)
	}

	if exp := 1; strings.Count(top, full) != exp*profileCell {
		t.Errorf("expected %d hour in the top row, got %q", exp, top)
	}
}

// A day that draws the same in every hour has no busiest hour to pick out of
// it, so it is left unmarked rather than given a pointer over all 24.
func TestProfileGraphLeavesAFlatDayUnmarked(t *testing.T) {
	var b strings.Builder
	if err := ProfileGraph(&b, []estate.Cluster{{Name: "a", Peak: 5}}, Bands{Base: true, Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(b.String(), profileMarker) {
		t.Errorf("expected no markers on a flat day, got:\n%s", b.String())
	}

	if strings.Contains(b.String(), profileBusiestLabel) {
		t.Errorf("expected the legend not to key a marker it doesn't draw, got:\n%s", b.String())
	}
}

func TestProfileGraphRejectsAChartWithNoBands(t *testing.T) {
	var b strings.Builder

	err := ProfileGraph(&b, profileFixture, Bands{})
	if !errors.Is(err, errNoBands) {
		t.Fatalf("expected the empty chart to be refused, got %v", err)
	}

	if b.String() != "" {
		t.Errorf("expected nothing to be written, got:\n%s", b.String())
	}
}

// An estate that draws nothing has no axis to scale, so it says so rather than
// dividing by zero.
func TestProfileGraphWithNoDemand(t *testing.T) {
	var b strings.Builder

	idle := []estate.Cluster{{Name: "a", Provisioned: 10}}
	if err := ProfileGraph(&b, idle, Bands{Base: true, Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(b.String(), emptyProfile) {
		t.Errorf("expected %q, got:\n%s", emptyProfile, b.String())
	}
}

func TestProfileGraphReportsWriteErrors(t *testing.T) {
	err := ProfileGraph(failingWriter{}, profileFixture, Bands{Base: true, Peak: true})
	if !errors.Is(err, errWrite) {
		t.Fatalf("expected the write error to surface, got %v", err)
	}
}
