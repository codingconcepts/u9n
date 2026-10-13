package render

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/codingconcepts/u9n/internal/estate"
)

// peakFixture is two clusters whose peaks fall in different hours, so the two
// sizings differ: 150 if they peaked at once, and 110 in the hour they share,
// which is the big one at peak and the small one at its base.
var peakFixture = []estate.Cluster{
	{Name: "big", Provisioned: 200, Peak: 100, PeakFrom: 0, PeakTo: 1, Windowed: true},
	{Name: "small", Provisioned: 100, Peak: 50, Base: 10, PeakFrom: 12, PeakTo: 13, Windowed: true},
}

// peakSummary sizes a set of clusters the way the peak command does, which is
// without headroom or growth, since the chart draws demand rather than what it
// would be racked against.
func peakSummary(clusters []estate.Cluster) estate.Summary {
	return estate.Summarise(clusters, estate.Assumptions{Years: 1})
}

func TestPeak(t *testing.T) {
	var b strings.Builder
	if err := Peak(&b, peakSummary(peakFixture)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// gutter is the name column and the bar beside it, which the figures are
	// printed clear of: the longest of "small" and "Total", two spaces, and
	// the bar itself.
	gutter := 5 + 2 + peakBarWidth

	exp := strings.Join([]string{
		"",
		// The sum of the peaks is a sizing of its own, so it is drawn whole
		// rather than split into the two bands the rows below it are.
		"Sum of the peaks  " + strings.Repeat("█", 40) + "  150",
		"",
		"Peak of the sums  " + strings.Repeat("█", 30) + strings.Repeat(" ", 10) + "  110",
		"",
		"Avoided           " + strings.Repeat(" ", 30) + strings.Repeat("░", 10) + "  40 (26.67%)",
		"",
		"",
		fmt.Sprintf(peakHeading, "00:00"),
		"",
		// The figures are headed over their own columns, clear of the bars, and
		// are ordered the way the totals above them are.
		strings.Repeat(" ", gutter) + "  Sum of peaks  Peak of sums",
		"big    " + strings.Repeat("█", 40) + "           100           100",
		"small  " + strings.Repeat("█", 4) + strings.Repeat("░", 16) + strings.Repeat(" ", 20) + "            50            10",
		"Total" + strings.Repeat(" ", gutter-len("Total")) + "           150           110",
		"",
		// The same comparison read across the day rather than in the hour the
		// rows above were taken from, and against both racks rather than one.
		// The 22 hours the estate spends at its floor are counted rather than
		// listed.
		spreadHeading,
		"",
		"          Hours     Drawn  Idle sum of peaks  Idle peak of sums",
		"Busiest   00:00       110        40 (26.67%)          0 (0.00%)",
		"Average   all day      16       134 (89.33%)        94 (85.45%)",
		"Quietest  22 hours     10       140 (93.33%)       100 (90.91%)",
		"",
		"",
	}, "\n")

	if b.String() != exp {
		t.Errorf("expected:\n%s\ngot:\n%s", exp, b.String())
	}
}

// The clusters are listed busiest first, whatever order the config put them
// in, so the bars that account for most of the gap are read first.
func TestPeakListsTheBiggestClustersFirst(t *testing.T) {
	clusters := []estate.Cluster{
		{Name: "small", Peak: 10, PeakFrom: 1, PeakTo: 2, Windowed: true},
		{Name: "big", Peak: 100, PeakFrom: 3, PeakTo: 4, Windowed: true},
		{Name: "middle", Peak: 50, PeakFrom: 5, PeakTo: 6, Windowed: true},
	}

	var b strings.Builder
	if err := Peak(&b, peakSummary(clusters)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var order []string

	for line := range strings.SplitSeq(b.String(), "\n") {
		for _, name := range []string{"big", "middle", "small"} {
			if strings.HasPrefix(line, name) {
				order = append(order, name)
			}
		}
	}

	if exp := []string{"big", "middle", "small"}; !slices.Equal(order, exp) {
		t.Errorf("expected the clusters in the order %v, got %v", exp, order)
	}
}

// A cluster that is a rounding error next to the biggest one still has both
// parts of its bar drawn, or the chart would say it is wholly avoided when it
// isn't.
func TestPeakDrawsBothPartsOfATinyCluster(t *testing.T) {
	clusters := []estate.Cluster{
		{Name: "big", Peak: 1000, PeakFrom: 0, PeakTo: 1, Windowed: true},
		{Name: "tiny", Peak: 10, Base: 2, PeakFrom: 12, PeakTo: 13, Windowed: true},
	}

	var b strings.Builder
	if err := Peak(&b, peakSummary(clusters)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var row string

	for line := range strings.SplitSeq(b.String(), "\n") {
		if strings.HasPrefix(line, "tiny") {
			row = line
		}
	}

	if !strings.Contains(row, full) || !strings.Contains(row, shade) {
		t.Errorf("expected the tiny cluster to keep a cell of each band, got %q", row)
	}
}

// Peak hours beyond the first draw the same total, so the heading names every
// hour the comparison holds in rather than only the one the rows came from.
func TestPeakNamesTheOtherPeakHours(t *testing.T) {
	var b strings.Builder

	// Two hours of peak, so the estate is as busy at 01:00 as it is at 00:00.
	clusters := []estate.Cluster{{Name: "a", Peak: 10, Base: 1, PeakFrom: 0, PeakTo: 2, Windowed: true}}
	if err := Peak(&b, peakSummary(clusters)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exp := fmt.Sprintf(peakHeading, "00:00, 01:00"); !strings.Contains(b.String(), exp) {
		t.Errorf("expected %q, got:\n%s", exp, b.String())
	}
}

// A day with no shape to it draws the same in its busiest hour as in its
// quietest, so the spread has three rows of one figure to show and is left
// off rather than printed.
func TestPeakWithoutASpread(t *testing.T) {
	var b strings.Builder

	clusters := []estate.Cluster{{Name: "a", Peak: 10}, {Name: "b", Peak: 5}}
	if err := Peak(&b, peakSummary(clusters)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(b.String(), spreadHeading) {
		t.Errorf("expected no spread for a flat day, got:\n%s", b.String())
	}
}

// Without peak windows the two sizings are the same number, which is a fact
// about the config rather than about the estate, so the chart says so.
func TestPeakWithoutWindows(t *testing.T) {
	var b strings.Builder

	clusters := []estate.Cluster{{Name: "a", Peak: 10}, {Name: "b", Peak: 5}}
	if err := Peak(&b, peakSummary(clusters)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(b.String(), noWindows) {
		t.Errorf("expected %q, got:\n%s", noWindows, b.String())
	}

	if !strings.Contains(b.String(), "0 (0.00%)") {
		t.Errorf("expected nothing to be avoided, got:\n%s", b.String())
	}
}

// An estate that draws nothing has no bar to scale, so it says so rather than
// dividing by zero.
func TestPeakWithNoDemand(t *testing.T) {
	var b strings.Builder

	if err := Peak(&b, peakSummary([]estate.Cluster{{Name: "a", Provisioned: 10}})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(b.String(), emptyPeak) {
		t.Errorf("expected %q, got:\n%s", emptyPeak, b.String())
	}
}

func TestPeakReportsWriteErrors(t *testing.T) {
	if err := Peak(failingWriter{}, peakSummary(peakFixture)); !errors.Is(err, errWrite) {
		t.Fatalf("expected the write error to surface, got %v", err)
	}
}
