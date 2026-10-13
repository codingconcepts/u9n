package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/codingconcepts/u9n/internal/estate"
)

// profileFixture is a single cluster peaking for two hours, so the chart has a
// base in all 24 hours and an uplift in two of them.
var profileFixture = []estate.Cluster{
	{Name: "a", Provisioned: 10, Peak: 5, Base: 1, PeakFrom: 9, PeakTo: 11, Windowed: true},
}

func TestProfile(t *testing.T) {
	var b strings.Builder
	if err := Profile(&b, profileFixture, Bands{Base: true, Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The uplift reaches the top of the chart in the two peak hours, and the
	// base covers the foot of all 24.
	exp := strings.Join([]string{
		"",
		"░░ Base   ██ Peak",
		"",
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
func TestProfileDrawsOnlyTheBandsAskedFor(t *testing.T) {
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
			if err := Profile(&b, profileFixture, c.bands); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			lines := strings.Split(b.String(), "\n")

			// The key comes first, with a blank line either side of it, so the
			// fourth line is the tallest row of the chart, which carries the
			// number the axis is scaled to.
			if top := lines[3]; !strings.HasPrefix(top, c.expTop) {
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

func TestProfileRejectsAChartWithNoBands(t *testing.T) {
	var b strings.Builder

	err := Profile(&b, profileFixture, Bands{})
	if !errors.Is(err, errNoBands) {
		t.Fatalf("expected the empty chart to be refused, got %v", err)
	}

	if b.String() != "" {
		t.Errorf("expected nothing to be written, got:\n%s", b.String())
	}
}

// An estate that draws nothing has no axis to scale, so it says so rather than
// dividing by zero.
func TestProfileWithNoDemand(t *testing.T) {
	var b strings.Builder

	idle := []estate.Cluster{{Name: "a", Provisioned: 10}}
	if err := Profile(&b, idle, Bands{Base: true, Peak: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(b.String(), emptyProfile) {
		t.Errorf("expected %q, got:\n%s", emptyProfile, b.String())
	}
}

func TestProfileReportsWriteErrors(t *testing.T) {
	err := Profile(failingWriter{}, profileFixture, Bands{Base: true, Peak: true})
	if !errors.Is(err, errWrite) {
		t.Fatalf("expected the write error to surface, got %v", err)
	}
}
