package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/codingconcepts/u9n/internal/estate"
)

var errWrite = errors.New("no room on the page")

// fixture is a single cluster peaking for two hours, chosen so every figure in
// the report is small enough to check by hand.
func fixture(a estate.Assumptions) estate.Summary {
	return estate.Summarise([]estate.Cluster{
		{Name: "a", Provisioned: 10, Peak: 5, Base: 1, PeakFrom: 9, PeakTo: 11, Windowed: true},
	}, a)
}

func TestTable(t *testing.T) {
	var b strings.Builder
	if err := Table(&b, fixture(estate.Assumptions{Headroom: 20, Years: 1})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	exp := strings.Join([]string{
		"Clusters:          1",
		"Provisioned vCPU:  10",
		"Headroom:          20%",
		"Peak diversity:    0.00%",
		"",
		"               Current    Worst case  Hour aware",
		"Required vCPU  10         6           6",
		"Peak vCPU      5          5           5",
		"Utilisation    50.00%     83.33%      83.33%",
		"Peak hours     -          -           09:00, 10:00",
		"Saved vCPU     0 (0.00%)  4 (40.00%)  4 (40.00%)",
		"",
	}, "\n")

	if b.String() != exp {
		t.Errorf("expected:\n%s\ngot:\n%s", exp, b.String())
	}
}

// Growth only earns a row when it actually moves the sizing.
func TestTableReportsGrowthWhenItIsSet(t *testing.T) {
	var plain, grown strings.Builder

	if err := Table(&plain, fixture(estate.Assumptions{Headroom: 20, Years: 1})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := Table(&grown, fixture(estate.Assumptions{Headroom: 20, Growth: 10, Years: 3})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(plain.String(), "Growth:") {
		t.Error("expected no growth row when growth leaves the sizing alone")
	}

	if exp := "10% a year over 3 years (33.10% in total)"; !strings.Contains(grown.String(), exp) {
		t.Errorf("expected the growth row %q, got:\n%s", exp, grown.String())
	}
}

// Without peak windows there is no diversity to report, but the hour aware
// column still appears, repeating the worst case and carrying the peak hours,
// which are every hour of a day that never lets up.
func TestTableWithoutWindows(t *testing.T) {
	s := estate.Summarise([]estate.Cluster{{Name: "a", Provisioned: 10, Peak: 5}}, estate.Assumptions{Headroom: 20, Years: 1})

	var b strings.Builder
	if err := Table(&b, s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(b.String(), "Peak diversity") {
		t.Errorf("expected no %q in:\n%s", "Peak diversity", b.String())
	}

	for _, want := range []string{"Hour aware", "all day"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("expected %q in:\n%s", want, b.String())
		}
	}
}

func TestTableReportsWriteErrors(t *testing.T) {
	if err := Table(failingWriter{}, fixture(estate.Assumptions{Headroom: 20, Years: 1})); err == nil {
		t.Fatal("expected an error from a writer that refuses to write")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWrite
}
