package estate

import (
	"math"
	"slices"
	"testing"
)

func TestSummarise(t *testing.T) {
	staggered := []Cluster{
		{Provisioned: 12, Peak: 10, Base: 2, PeakFrom: 2, PeakTo: 3, Windowed: true},
		{Provisioned: 12, Peak: 10, Base: 2, PeakFrom: 10, PeakTo: 11, Windowed: true},
		{Provisioned: 12, Peak: 10, Base: 2, PeakFrom: 18, PeakTo: 19, Windowed: true},
	}

	// allDay is every hour of the clock, which is what peaks in a day that is
	// as busy at one hour as it is at any other.
	allDay := make([]int, 0, HoursPerDay)
	for hour := range HoursPerDay {
		allDay = append(allDay, hour)
	}

	cases := []struct {
		name         string
		clusters     []Cluster
		assumptions  Assumptions
		expPeak      map[string]float64
		expRequired  map[string]float64
		expSaved     map[string]float64
		expPeakHours []int
	}{
		{
			name:        "without windows peaks are assumed to coincide",
			clusters:    []Cluster{{Provisioned: 4, Peak: 1}, {Provisioned: 12, Peak: 5}},
			assumptions: Assumptions{Headroom: 20, Years: 1},
			expPeak:     map[string]float64{KeyCurrent: 6, KeyWorstCase: 6, KeyHourAware: 6},
			// ceil(6 * 1.2) == ceil(7.2)
			expRequired: map[string]float64{KeyCurrent: 16, KeyWorstCase: 8, KeyHourAware: 8},
			expSaved:    map[string]float64{KeyCurrent: 0, KeyWorstCase: 8, KeyHourAware: 8},
			// No windows, so every hour draws the same and every hour ties.
			expPeakHours: allDay,
		},
		{
			name:        "staggered windows lower the hour aware sizing",
			clusters:    staggered,
			assumptions: Assumptions{Headroom: 20, Years: 1},
			expPeak:     map[string]float64{KeyCurrent: 30, KeyWorstCase: 30, KeyHourAware: 14}, // 10 + 2 + 2
			// ceil(30 * 1.2) and ceil(14 * 1.2)
			expRequired:  map[string]float64{KeyCurrent: 36, KeyWorstCase: 36, KeyHourAware: 17},
			expSaved:     map[string]float64{KeyCurrent: 0, KeyWorstCase: 0, KeyHourAware: 19},
			expPeakHours: []int{2, 10, 18},
		},
		{
			name:        "growth compounds over the given years",
			clusters:    staggered,
			assumptions: Assumptions{Growth: 10, Years: 3},
			expPeak:     map[string]float64{KeyCurrent: 30, KeyWorstCase: 30, KeyHourAware: 14},
			// 1.1^3 == 1.331, so ceil(30 * 1.331) and ceil(14 * 1.331)
			expRequired:  map[string]float64{KeyCurrent: 36, KeyWorstCase: 40, KeyHourAware: 19},
			expSaved:     map[string]float64{KeyCurrent: 0, KeyWorstCase: -4, KeyHourAware: 17},
			expPeakHours: []int{2, 10, 18},
		},
		{
			name:         "negative growth shrinks the estate",
			clusters:     staggered,
			assumptions:  Assumptions{Growth: -50, Years: 1},
			expPeak:      map[string]float64{KeyCurrent: 30, KeyWorstCase: 30, KeyHourAware: 14},
			expRequired:  map[string]float64{KeyCurrent: 36, KeyWorstCase: 15, KeyHourAware: 7},
			expSaved:     map[string]float64{KeyCurrent: 0, KeyWorstCase: 21, KeyHourAware: 29},
			expPeakHours: []int{2, 10, 18},
		},
		{
			name: "overlapping windows stack",
			clusters: []Cluster{
				{Provisioned: 12, Peak: 10, Base: 1, PeakFrom: 9, PeakTo: 12, Windowed: true},
				{Provisioned: 12, Peak: 10, Base: 1, PeakFrom: 11, PeakTo: 14, Windowed: true},
			},
			assumptions:  Assumptions{Years: 1},
			expPeak:      map[string]float64{KeyCurrent: 20, KeyWorstCase: 20, KeyHourAware: 20},
			expRequired:  map[string]float64{KeyCurrent: 24, KeyWorstCase: 20, KeyHourAware: 20},
			expSaved:     map[string]float64{KeyCurrent: 0, KeyWorstCase: 4, KeyHourAware: 4},
			expPeakHours: []int{11},
		},
		{
			name:         "zero provisioned",
			clusters:     []Cluster{{Provisioned: 0, Peak: 0}},
			assumptions:  Assumptions{Headroom: 50, Years: 1},
			expPeak:      map[string]float64{KeyCurrent: 0, KeyWorstCase: 0, KeyHourAware: 0},
			expRequired:  map[string]float64{KeyCurrent: 0, KeyWorstCase: 0, KeyHourAware: 0},
			expSaved:     map[string]float64{KeyCurrent: 0, KeyWorstCase: 0, KeyHourAware: 0},
			expPeakHours: allDay,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Summarise(c.clusters, c.assumptions)

			for _, sc := range []Scenario{s.Current, s.WorstCase, s.HourAware} {
				if sc.Peak != c.expPeak[sc.Key] {
					t.Errorf("%s: expected peak %g, got %g", sc.Key, c.expPeak[sc.Key], sc.Peak)
				}

				if sc.Required != c.expRequired[sc.Key] {
					t.Errorf("%s: expected required %g, got %g", sc.Key, c.expRequired[sc.Key], sc.Required)
				}

				if sc.Saved() != c.expSaved[sc.Key] {
					t.Errorf("%s: expected saved %g, got %g", sc.Key, c.expSaved[sc.Key], sc.Saved())
				}
			}

			if !slices.Equal(s.HourAware.PeakHours, c.expPeakHours) {
				t.Errorf("expected peak hours %v, got %v", c.expPeakHours, s.HourAware.PeakHours)
			}
		})
	}
}

// Utilisation is peak demand against the capacity each sizing stands up, not
// against today's estate, so a consolidated sizing runs hotter than the status
// quo it replaces.
func TestUtilisationIsRelativeToItsOwnSizing(t *testing.T) {
	clusters := []Cluster{
		{Provisioned: 100, Peak: 40, Base: 10, PeakFrom: 9, PeakTo: 17, Windowed: true},
		{Provisioned: 100, Peak: 40, Base: 10, PeakFrom: 20, PeakTo: 23, Windowed: true},
	}

	s := Summarise(clusters, Assumptions{Headroom: 25, Years: 1})

	// Today: 80 of peak demand against 200 racked.
	if exp := 40.0; s.Current.Utilisation() != exp {
		t.Errorf("current: expected utilisation %g, got %g", exp, s.Current.Utilisation())
	}

	// Worst case: 80 of peak against ceil(80 * 1.25) == 100.
	if exp := 80.0; s.WorstCase.Utilisation() != exp {
		t.Errorf("worst case: expected utilisation %g, got %g", exp, s.WorstCase.Utilisation())
	}

	// Hour aware: peaks never coincide, so 50 of peak against ceil(50 * 1.25)
	// == 63, which is the rounding up of the host leaving a little slack.
	if exp := 50.0 / 63 * 100; s.HourAware.Utilisation() != exp {
		t.Errorf("hour aware: expected utilisation %g, got %g", exp, s.HourAware.Utilisation())
	}

	// Headroom is what holds utilisation off 100%, so dropping it entirely
	// should saturate the consolidated sizing.
	saturated := Summarise(clusters, Assumptions{Years: 1})
	if exp := 100.0; saturated.WorstCase.Utilisation() != exp {
		t.Errorf("worst case without headroom: expected utilisation %g, got %g", exp, saturated.WorstCase.Utilisation())
	}
}

// An empty estate has nothing to divide by, and must report zero rather than
// NaN.
func TestEmptyEstateHasNoPercentages(t *testing.T) {
	s := Summarise([]Cluster{{Name: "a"}}, Assumptions{Headroom: 20, Years: 1})

	for _, sc := range s.Scenarios() {
		if got := sc.Utilisation(); got != 0 {
			t.Errorf("%s: expected utilisation 0, got %g", sc.Key, got)
		}

		if got := sc.SavedPercent(); got != 0 {
			t.Errorf("%s: expected saved percent 0, got %g", sc.Key, got)
		}
	}

	if got := s.Diversity(); got != 0 {
		t.Errorf("expected diversity 0, got %g", got)
	}
}

// The current sizing is the status quo, so it must never move with headroom or
// growth. Everything is measured against it.
func TestCurrentIsAFixedBaseline(t *testing.T) {
	clusters := []Cluster{{Provisioned: 100, Peak: 40, Base: 10, PeakFrom: 9, PeakTo: 17, Windowed: true}}

	for _, headroom := range []float64{0, 20, 500} {
		for _, growth := range []float64{-50, 0, 25} {
			s := Summarise(clusters, Assumptions{Headroom: headroom, Growth: growth, Years: 5})

			if s.Current.Required != 100 {
				t.Fatalf("headroom %g growth %g: expected required 100, got %g", headroom, growth, s.Current.Required)
			}

			if s.Current.Saved() != 0 {
				t.Fatalf("headroom %g growth %g: expected saved 0, got %g", headroom, growth, s.Current.Saved())
			}
		}
	}
}

func TestScenarios(t *testing.T) {
	windowed := Summarise([]Cluster{{Provisioned: 10, Peak: 5, PeakFrom: 9, PeakTo: 17, Windowed: true}}, Assumptions{Headroom: 20, Years: 1})
	if got := len(windowed.Scenarios()); got != 3 {
		t.Errorf("expected 3 scenarios for a windowed config, got %d", got)
	}

	// Without windows the hour aware sizing is the worst case, but it is still
	// reported so the peak hours have a column to sit in.
	plain := Summarise([]Cluster{{Provisioned: 10, Peak: 5}}, Assumptions{Headroom: 20, Years: 1})
	if got := len(plain.Scenarios()); got != 3 {
		t.Errorf("expected 3 scenarios for a config without windows, got %d", got)
	}

	hourAware := plain.Scenarios()[2]
	if hourAware.Required != plain.WorstCase.Required || hourAware.Peak != plain.WorstCase.Peak {
		t.Errorf("expected the hour aware sizing to match the worst case, got %+v", hourAware)
	}

	if len(hourAware.PeakHours) != HoursPerDay {
		t.Errorf("expected a flat day to peak in every hour, got %v", hourAware.PeakHours)
	}
}

func TestDiversity(t *testing.T) {
	s := Summarise([]Cluster{
		{Provisioned: 10, Peak: 10, PeakFrom: 1, PeakTo: 2, Windowed: true},
		{Provisioned: 10, Peak: 10, PeakFrom: 5, PeakTo: 6, Windowed: true},
	}, Assumptions{Years: 1})

	// Peaks never coincide and there is no base load, so half the worst case
	// total is avoided.
	if s.Diversity() != 50 {
		t.Errorf("expected diversity 50, got %g", s.Diversity())
	}
}

func TestAssumptionsValidate(t *testing.T) {
	cases := []struct {
		name        string
		assumptions Assumptions
		expErr      string
	}{
		{name: "defaults", assumptions: Assumptions{Headroom: 20, Years: 1}},
		{name: "no headroom at all", assumptions: Assumptions{}},
		{name: "negative headroom", assumptions: Assumptions{Headroom: -1}, expErr: "headroom must not be negative"},
		{name: "growth of -100", assumptions: Assumptions{Growth: -100}, expErr: "growth must be greater than -100"},
		{name: "growth below -100", assumptions: Assumptions{Growth: -101}, expErr: "growth must be greater than -100"},
		{name: "negative years", assumptions: Assumptions{Years: -1}, expErr: "years must not be negative"},
		{
			name:        "headroom of NaN",
			assumptions: Assumptions{Headroom: math.NaN()},
			expErr:      "headroom must be a finite number",
		},
		{
			name:        "infinite growth",
			assumptions: Assumptions{Growth: math.Inf(1)},
			expErr:      "growth must be a finite number",
		},
		{
			name:        "infinite years",
			assumptions: Assumptions{Years: math.Inf(-1)},
			expErr:      "years must be a finite number",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.assumptions.Validate()

			switch {
			case c.expErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case c.expErr != "" && (err == nil || err.Error() != c.expErr):
				t.Fatalf("expected error %q, got %v", c.expErr, err)
			}
		})
	}
}
