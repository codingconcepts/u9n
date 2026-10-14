package render

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/codingconcepts/u9n/internal/estate"
	"github.com/codingconcepts/u9n/internal/text"
)

func TestCompare(t *testing.T) {
	var b strings.Builder
	if err := Compare(&b, peakSummary(peakFixture)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The two clusters come to 150 between them, so the bar is split 26 cells
	// to 13 of the 39 the separator between the segments leaves of it, and
	// every bar is padded out to the 40 the total takes.
	exp := strings.Join([]string{
		"",
		compareHeading,
		"",
		"░░ Base   ██ Peak   ██ Together",
		"",
		"big",
		strings.Repeat(" ", 40) + "    0",
		strings.Repeat("█", 26) + strings.Repeat(" ", 14) + "  100",
		"",
		"small",
		strings.Repeat("░", 3) + strings.Repeat(" ", 37) + "   10",
		strings.Repeat("█", 13) + strings.Repeat(" ", 27) + "   50",
		"",
		compareTotal,
		"",
		// The segments are the bars above laid end to end, with only the
		// total printed against them.
		strings.Repeat("█", 26) + " " + strings.Repeat("█", 13) + "  150",
		"",
		// The same segments cut back to the hour the clusters share, where the
		// big one is at its peak and the small one is at its base, so the row
		// comes up short of the one above it by what shared hardware never has
		// to stand up.
		fmt.Sprintf(compareTogether, "00:00"),
		"",
		strings.Repeat("█", 26) + " " + strings.Repeat("█", 3) + strings.Repeat(" ", 10) + "  110",
		"",
		// The note is wrapped to the width the prose is set in once the
		// figures are in it, rather than at a line break written into it, so
		// it is taken at that width here rather than broken by hand.
		text.Wrap("The 40 (26.67%) between them is racked for peaks that never land in "+
			"the same hour. The peak command charts the gap.", text.Width),
		"",
		"",
	}, "\n")

	if b.String() != exp {
		t.Errorf("expected:\n%s\ngot:\n%s", exp, b.String())
	}
}

// The total is the one number the chart is scaled against, so the segments
// have to add up to it exactly however the shares round.
func TestCompareSegmentsFillTheBar(t *testing.T) {
	clusters := []estate.Cluster{
		{Name: "a", Peak: 33},
		{Name: "b", Peak: 33},
		{Name: "c", Peak: 33},
		{Name: "d", Peak: 1},
	}

	cells := compareCells(clusters, 100)

	// Three clusters leave 37 cells of the 40 for the bars themselves, the
	// other three going to the separators between the four segments.
	if got := compareBlocks(cells); got != 37 {
		t.Errorf("expected the segments to fill the bar, got %d cells from %v", got, cells)
	}

	if got := compareWidth(cells); got != peakBarWidth {
		t.Errorf("expected the row to come to %d columns, got %d", peakBarWidth, got)
	}
}

// A cluster the total barely notices is still a cluster that needs hardware,
// so it keeps a cell rather than rounding away to nothing.
func TestCompareDrawsATinyCluster(t *testing.T) {
	clusters := []estate.Cluster{
		{Name: "big", Peak: 10000},
		{Name: "tiny", Peak: 1},
	}

	cells := compareCells(clusters, 10001)
	if cells[1] != 1 {
		t.Errorf("expected the tiny cluster to keep a cell, got %v", cells)
	}

	if got, exp := compareBlocks(cells), 39; got != exp {
		t.Errorf("expected the cell to be taken off the big one, got %d cells from %v", got, cells)
	}
}

// More clusters than the bar has cells for is the one case the row is allowed
// past its usual width, since a cluster drawn as nothing at all isn't worth
// keeping the row narrow for.
func TestCompareWidensForAnEstateOfManyClusters(t *testing.T) {
	clusters := make([]estate.Cluster, 0, 30)
	for range 30 {
		clusters = append(clusters, estate.Cluster{Name: "c", Peak: 1})
	}

	cells := compareCells(clusters, 30)

	for i, c := range cells {
		if c != 1 {
			t.Fatalf("expected every cluster to keep a cell, got %d at %d of %v", c, i, cells)
		}
	}

	if got, exp := compareWidth(cells), 59; got != exp {
		t.Errorf("expected a row of %d columns, got %d", exp, got)
	}
}

// The clusters are listed biggest first, whatever order the config put them
// in, so the segments of the total run in the order the rows above it do.
func TestCompareListsTheBiggestClustersFirst(t *testing.T) {
	clusters := []estate.Cluster{
		{Name: "small", Peak: 10},
		{Name: "big", Peak: 100},
		{Name: "middle", Peak: 50},
	}

	var b strings.Builder
	if err := Compare(&b, peakSummary(clusters)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var order []string

	for line := range strings.SplitSeq(b.String(), "\n") {
		for _, name := range []string{"big", "middle", "small"} {
			if line == name {
				order = append(order, name)
			}
		}
	}

	if exp := "big middle small"; strings.Join(order, " ") != exp {
		t.Errorf("expected the clusters in the order %q, got %q", exp, strings.Join(order, " "))
	}
}

// The hour the clusters share doesn't come in the order their peaks do, since
// a cluster with a big peak sitting at its base draws less in that hour than a
// smaller one inside its window. The row is still drawn longest first.
func TestCompareSortsTheTotalsSegments(t *testing.T) {
	clusters := []estate.Cluster{
		// At 00:00 the big cluster is outside its window and holds a tenth of
		// its peak, while the small one is at all of its own.
		{Name: "big", Peak: 100, Base: 10, PeakFrom: 12, PeakTo: 13, Windowed: true},
		{Name: "small", Peak: 50, PeakFrom: 0, PeakTo: 1, Windowed: true},
	}

	var b strings.Builder
	if err := Compare(&b, peakSummary(clusters)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for line := range strings.SplitSeq(b.String(), "\n") {
		runs := strings.Fields(strings.TrimSpace(strings.SplitN(line, peakGap, 2)[0]))
		if len(runs) < 2 || !strings.Contains(runs[0], full) {
			continue
		}

		for i := 1; i < len(runs); i++ {
			if len(runs[i]) > len(runs[i-1]) {
				t.Errorf("expected the segments longest first, got %q", line)
			}
		}
	}
}

// Without peak windows every cluster counts as at peak in every hour, so the
// two totals are the same bar twice over and only the first is drawn.
func TestCompareWithoutWindows(t *testing.T) {
	var b strings.Builder

	clusters := []estate.Cluster{{Name: "a", Peak: 10}, {Name: "b", Peak: 5}}
	if err := Compare(&b, peakSummary(clusters)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, unwanted := range []string{"draw at once", togetherLabel, "between them"} {
		if strings.Contains(b.String(), unwanted) {
			t.Errorf("expected no %q for a config with no windows, got:\n%s", unwanted, b.String())
		}
	}
}

// Windows that all fall in the same hours leave nothing to avoid, so the
// second total would be the first one over again.
func TestCompareWithoutAGap(t *testing.T) {
	var b strings.Builder

	clusters := []estate.Cluster{
		{Name: "a", Peak: 10, Base: 1, PeakFrom: 9, PeakTo: 11, Windowed: true},
		{Name: "b", Peak: 5, Base: 1, PeakFrom: 9, PeakTo: 11, Windowed: true},
	}

	if err := Compare(&b, peakSummary(clusters)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(b.String(), togetherLabel) {
		t.Errorf("expected no second total when the clusters peak together, got:\n%s", b.String())
	}
}

// A cluster outside its window at the busiest hour is still drawing its base,
// so it keeps a segment of the second total rather than dropping out of it.
func TestCompareCountsTheBaseOfAnOffPeakCluster(t *testing.T) {
	clusters := []estate.Cluster{
		{Name: "big", Peak: 100, PeakFrom: 0, PeakTo: 1, Windowed: true},
		{Name: "small", Peak: 50, Base: 10, PeakFrom: 12, PeakTo: 13, Windowed: true},
	}

	// 26 cells to the big cluster and 13 to the small one, and at 00:00 the
	// big one is at its peak while the small one holds a fifth of its own.
	cells := compareHourCells(clusters, []int{26, 13}, 0)

	if exp := []int{26, 3}; !slices.Equal(cells, exp) {
		t.Errorf("expected %v cells, got %v", exp, cells)
	}
}

// A cluster whose base is a rounding error against its peak still draws that
// base at the busiest hour, so its segment is a cell rather than nothing.
func TestCompareKeepsACellForATinyBase(t *testing.T) {
	clusters := []estate.Cluster{
		{Name: "big", Peak: 1000, Base: 1, PeakFrom: 0, PeakTo: 1, Windowed: true},
	}

	if cells := compareHourCells(clusters, []int{40}, 12); cells[0] != 1 {
		t.Errorf("expected a cell for the base, got %v", cells)
	}
}

// An estate that peaks at nothing has no bar to scale, so it says so rather
// than dividing by zero.
func TestCompareWithNoDemand(t *testing.T) {
	var b strings.Builder

	if err := Compare(&b, peakSummary([]estate.Cluster{{Name: "a", Provisioned: 10}})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(b.String(), emptyCompare) {
		t.Errorf("expected %q, got:\n%s", emptyCompare, b.String())
	}
}

func TestCompareReportsWriteErrors(t *testing.T) {
	if err := Compare(failingWriter{}, peakSummary(peakFixture)); !errors.Is(err, errWrite) {
		t.Errorf("expected the write error, got %v", err)
	}
}
