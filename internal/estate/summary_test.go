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
			expPeak:     map[string]float64{KeyRackedToday: 6, KeySumOfPeaks: 6, KeyPeakOfSum: 6},
			// ceil(6 * 1.2) == ceil(7.2)
			expRequired: map[string]float64{KeyRackedToday: 16, KeySumOfPeaks: 8, KeyPeakOfSum: 8},
			expSaved:    map[string]float64{KeyRackedToday: 0, KeySumOfPeaks: 8, KeyPeakOfSum: 8},
			// No windows, so every hour draws the same and every hour ties.
			expPeakHours: allDay,
		},
		{
			name:        "staggered windows lower the peak of the sum",
			clusters:    staggered,
			assumptions: Assumptions{Headroom: 20, Years: 1},
			expPeak:     map[string]float64{KeyRackedToday: 30, KeySumOfPeaks: 30, KeyPeakOfSum: 14}, // 10 + 2 + 2
			// ceil(30 * 1.2) and ceil(14 * 1.2)
			expRequired:  map[string]float64{KeyRackedToday: 36, KeySumOfPeaks: 36, KeyPeakOfSum: 17},
			expSaved:     map[string]float64{KeyRackedToday: 0, KeySumOfPeaks: 0, KeyPeakOfSum: 19},
			expPeakHours: []int{2, 10, 18},
		},
		{
			name:        "growth compounds over the given years",
			clusters:    staggered,
			assumptions: Assumptions{Growth: 10, Years: 3},
			expPeak:     map[string]float64{KeyRackedToday: 30, KeySumOfPeaks: 30, KeyPeakOfSum: 14},
			// 1.1^3 == 1.331, so ceil(30 * 1.331) and ceil(14 * 1.331)
			expRequired:  map[string]float64{KeyRackedToday: 36, KeySumOfPeaks: 40, KeyPeakOfSum: 19},
			expSaved:     map[string]float64{KeyRackedToday: 0, KeySumOfPeaks: -4, KeyPeakOfSum: 17},
			expPeakHours: []int{2, 10, 18},
		},
		{
			name:         "negative growth shrinks the estate",
			clusters:     staggered,
			assumptions:  Assumptions{Growth: -50, Years: 1},
			expPeak:      map[string]float64{KeyRackedToday: 30, KeySumOfPeaks: 30, KeyPeakOfSum: 14},
			expRequired:  map[string]float64{KeyRackedToday: 36, KeySumOfPeaks: 15, KeyPeakOfSum: 7},
			expSaved:     map[string]float64{KeyRackedToday: 0, KeySumOfPeaks: 21, KeyPeakOfSum: 29},
			expPeakHours: []int{2, 10, 18},
		},
		{
			name: "overlapping windows stack",
			clusters: []Cluster{
				{Provisioned: 12, Peak: 10, Base: 1, PeakFrom: 9, PeakTo: 12, Windowed: true},
				{Provisioned: 12, Peak: 10, Base: 1, PeakFrom: 11, PeakTo: 14, Windowed: true},
			},
			assumptions:  Assumptions{Years: 1},
			expPeak:      map[string]float64{KeyRackedToday: 20, KeySumOfPeaks: 20, KeyPeakOfSum: 20},
			expRequired:  map[string]float64{KeyRackedToday: 24, KeySumOfPeaks: 20, KeyPeakOfSum: 20},
			expSaved:     map[string]float64{KeyRackedToday: 0, KeySumOfPeaks: 4, KeyPeakOfSum: 4},
			expPeakHours: []int{11},
		},
		{
			name:         "zero provisioned",
			clusters:     []Cluster{{Provisioned: 0, Peak: 0}},
			assumptions:  Assumptions{Headroom: 50, Years: 1},
			expPeak:      map[string]float64{KeyRackedToday: 0, KeySumOfPeaks: 0, KeyPeakOfSum: 0},
			expRequired:  map[string]float64{KeyRackedToday: 0, KeySumOfPeaks: 0, KeyPeakOfSum: 0},
			expSaved:     map[string]float64{KeyRackedToday: 0, KeySumOfPeaks: 0, KeyPeakOfSum: 0},
			expPeakHours: allDay,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Summarise(c.clusters, c.assumptions)

			for _, sc := range []Scenario{s.RackedToday, s.SumOfPeaks, s.PeakOfSum} {
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

			if !slices.Equal(s.PeakOfSum.PeakHours, c.expPeakHours) {
				t.Errorf("expected peak hours %v, got %v", c.expPeakHours, s.PeakOfSum.PeakHours)
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
	if exp := 40.0; s.RackedToday.Utilisation() != exp {
		t.Errorf("racked today: expected utilisation %g, got %g", exp, s.RackedToday.Utilisation())
	}

	// Sum of the peaks: 80 of peak against ceil(80 * 1.25) == 100.
	if exp := 80.0; s.SumOfPeaks.Utilisation() != exp {
		t.Errorf("sum of the peaks: expected utilisation %g, got %g", exp, s.SumOfPeaks.Utilisation())
	}

	// Peak of the sum: peaks never coincide, so 50 of peak against ceil(50 * 1.25)
	// == 63, which is the rounding up of the host leaving a little slack.
	if exp := 50.0 / 63 * 100; s.PeakOfSum.Utilisation() != exp {
		t.Errorf("peak of the sum: expected utilisation %g, got %g", exp, s.PeakOfSum.Utilisation())
	}

	// Headroom is what holds utilisation off 100%, so dropping it entirely
	// should saturate the consolidated sizing.
	saturated := Summarise(clusters, Assumptions{Years: 1})
	if exp := 100.0; saturated.SumOfPeaks.Utilisation() != exp {
		t.Errorf("sum of the peaks without headroom: expected utilisation %g, got %g", exp, saturated.SumOfPeaks.Utilisation())
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

// Racked today is the status quo, so it must never move with headroom or
// growth. Everything is measured against it.
func TestRackedTodayIsAFixedBaseline(t *testing.T) {
	clusters := []Cluster{{Provisioned: 100, Peak: 40, Base: 10, PeakFrom: 9, PeakTo: 17, Windowed: true}}

	for _, headroom := range []float64{0, 20, 500} {
		for _, growth := range []float64{-50, 0, 25} {
			s := Summarise(clusters, Assumptions{Headroom: headroom, Growth: growth, Years: 5})

			if s.RackedToday.Required != 100 {
				t.Fatalf("headroom %g growth %g: expected required 100, got %g", headroom, growth, s.RackedToday.Required)
			}

			if s.RackedToday.Saved() != 0 {
				t.Fatalf("headroom %g growth %g: expected saved 0, got %g", headroom, growth, s.RackedToday.Saved())
			}
		}
	}
}

func TestScenarios(t *testing.T) {
	windowed := Summarise([]Cluster{{Provisioned: 10, Peak: 5, PeakFrom: 9, PeakTo: 17, Windowed: true}}, Assumptions{Headroom: 20, Years: 1})
	if got := len(windowed.Scenarios()); got != 3 {
		t.Errorf("expected 3 scenarios for a windowed config, got %d", got)
	}

	// Without windows the peak of the sum is the sum of the peaks, but it is
	// still reported so the peak hours have a column to sit in.
	plain := Summarise([]Cluster{{Provisioned: 10, Peak: 5}}, Assumptions{Headroom: 20, Years: 1})
	if got := len(plain.Scenarios()); got != 3 {
		t.Errorf("expected 3 scenarios for a config without windows, got %d", got)
	}

	peakOfSum := plain.Scenarios()[2]
	if peakOfSum.Required != plain.SumOfPeaks.Required || peakOfSum.Peak != plain.SumOfPeaks.Peak {
		t.Errorf("expected the peak of the sum to match the sum of the peaks, got %+v", peakOfSum)
	}

	if len(peakOfSum.PeakHours) != HoursPerDay {
		t.Errorf("expected a flat day to peak in every hour, got %v", peakOfSum.PeakHours)
	}
}

func TestDiversity(t *testing.T) {
	s := Summarise([]Cluster{
		{Provisioned: 10, Peak: 10, PeakFrom: 1, PeakTo: 2, Windowed: true},
		{Provisioned: 10, Peak: 10, PeakFrom: 5, PeakTo: 6, Windowed: true},
	}, Assumptions{Years: 1})

	// Peaks never coincide and there is no base load, so half the sum of the peaks
	// total is avoided.
	if s.Diversity() != 50 {
		t.Errorf("expected diversity 50, got %g", s.Diversity())
	}
}

// The day long figures are the peak diversity read over 24 hours rather than
// in the one hour the estate is busiest, which is the hour a sum of the peaks
// rack wastes least of.
func TestDayLongFigures(t *testing.T) {
	// Two clusters of 10, each at peak for one hour of the day and at a base
	// of 1 for the other 23, so the estate draws 11 in two hours and 2 in the
	// other 22 against a sum of the peaks rack of 20.
	s := Summarise([]Cluster{
		{Provisioned: 10, Peak: 10, Base: 1, PeakFrom: 1, PeakTo: 2, Windowed: true},
		{Provisioned: 10, Peak: 10, Base: 1, PeakFrom: 5, PeakTo: 6, Windowed: true},
	}, Assumptions{Years: 1})

	// 11 + 11 + 22 * 2
	if exp, got := 66.0, s.VCPUHours(); got != exp {
		t.Errorf("expected %g vcpu hours, got %g", exp, got)
	}

	if exp, got := 480.0, s.SumOfPeaksVCPUHours(); got != exp {
		t.Errorf("expected %g sum of the peaks vcpu hours, got %g", exp, got)
	}

	if exp, got := 414.0, s.IdleVCPUHours(); got != exp {
		t.Errorf("expected %g idle vcpu hours, got %g", exp, got)
	}

	// 414 / 480 and 66 / 480, which are the two sides of the same split.
	if exp, got := 86.25, s.IdlePercent(); got != exp {
		t.Errorf("expected %g%% idle, got %g%%", exp, got)
	}

	// 66 / 480 doesn't land on a float exactly, so the share it is reported as
	// is only ever good to the two places it is printed to.
	if exp, got := 13.75, s.MeanUtilisation(); math.Abs(got-exp) > 0.005 {
		t.Errorf("expected %g%% mean utilisation, got %g%%", exp, got)
	}

	// The peak is held in the two hours a cluster is at peak in, and the floor
	// in the 22 hours neither is.
	if exp := []int{1, 5}; !slices.Equal(s.PeakOfSum.PeakHours, exp) {
		t.Errorf("expected the peak hours %v, got %v", exp, s.PeakOfSum.PeakHours)
	}

	if got := len(s.QuietHours()); got != 22 {
		t.Errorf("expected 22 quiet hours, got %d: %v", got, s.QuietHours())
	}

	if slices.Contains(s.QuietHours(), 1) || slices.Contains(s.QuietHours(), 5) {
		t.Errorf("expected the peak hours to be left out of the quiet hours, got %v", s.QuietHours())
	}
}

// A day with nothing in it divides by zero on the way to a percentage, so the
// day long figures have to answer it the way the rest of the summary does.
func TestDayLongFiguresOfAnEmptyEstate(t *testing.T) {
	s := Summarise([]Cluster{{Name: "a"}}, Assumptions{Years: 1})

	if got := s.IdlePercent(); got != 0 {
		t.Errorf("expected idle percent 0, got %g", got)
	}

	if got := s.MeanUtilisation(); got != 0 {
		t.Errorf("expected mean utilisation 0, got %g", got)
	}

	// Every hour draws nothing, so every hour ties at both ends of the day.
	if got := len(s.QuietHours()); got != HoursPerDay {
		t.Errorf("expected every hour to be a quiet hour, got %d", got)
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
