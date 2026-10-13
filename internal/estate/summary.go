package estate

import (
	"errors"
	"fmt"
	"math"
)

// Scenario keys, which are stable enough to key json output by.
const (
	KeyCurrent   = "current"
	KeyWorstCase = "worst_case"
	KeyHourAware = "hour_aware"
)

// Assumptions are the knobs a sizing is calculated under.
type Assumptions struct {
	// Headroom is the percentage of spare capacity left on top of peak demand.
	Headroom float64

	// Growth is the expected annual growth in demand as a percentage, and
	// Years the number of years it compounds over.
	Growth float64
	Years  float64
}

// Validate rejects assumptions the sizing can't be calculated under.
func (a Assumptions) Validate() error {
	// NaN and Inf parse happily as floats but compare false against every
	// bound below, so they have to be ruled out before the range checks.
	for _, f := range []struct {
		name  string
		value float64
	}{
		{"headroom", a.Headroom},
		{"growth", a.Growth},
		{"years", a.Years},
	} {
		if math.IsNaN(f.value) || math.IsInf(f.value, 0) {
			return fmt.Errorf("%s must be a finite number", f.name)
		}
	}

	switch {
	case a.Headroom < 0:
		return errors.New("headroom must not be negative")
	case a.Growth <= -100:
		return errors.New("growth must be greater than -100")
	case a.Years < 0:
		return errors.New("years must not be negative")
	}

	return nil
}

// Scenario is one way of answering "how much vcpu does the estate need", given
// an assumption about when the clusters peak.
type Scenario struct {
	Key      string
	Label    string
	Peak     float64
	Required float64

	Provisioned float64

	// PeakHours are the hours the estate is busiest in, in order. A day often
	// holds its peak for a stretch rather than a single hour, so every hour
	// tied for the busiest is reported. Only meaningful when the scenario is
	// hour aware.
	PeakHours []int
	HourAware bool

	// Baseline marks the status quo, which is sized by what is already
	// provisioned rather than by demand.
	Baseline bool
}

// Utilisation measures peak demand against the capacity this sizing would
// stand up, not against today's estate. For the status quo the two are the
// same thing; for a consolidated sizing they are not, and dividing by an
// estate you would no longer have understates how hard the new one runs.
func (s Scenario) Utilisation() float64 {
	if s.Required == 0 {
		return 0
	}

	return s.Peak / s.Required * 100
}

// Saved is the vcpu this sizing gives back against what is racked today.
func (s Scenario) Saved() float64 {
	return s.Provisioned - s.Required
}

// SavedPercent is Saved as a share of today's estate.
func (s Scenario) SavedPercent() float64 {
	if s.Provisioned == 0 {
		return 0
	}

	return s.Saved() / s.Provisioned * 100
}

// Summary is the whole estate under one set of assumptions.
type Summary struct {
	Assumptions

	Clusters     []Cluster
	Provisioned  float64
	GrowthFactor float64
	Hourly       [HoursPerDay]float64
	Current      Scenario
	WorstCase    Scenario
	HourAware    Scenario
	Windowed     bool
}

// Scenarios returns the sizings worth reporting. Without peak windows the hour
// aware sizing is identical to the worst case by definition, but it is still
// reported, so that the peak hour is always answered rather than left blank.
func (s Summary) Scenarios() []Scenario {
	return []Scenario{s.Current, s.WorstCase, s.HourAware}
}

// Diversity is the share of the worst case total avoided by peaks falling in
// different hours.
func (s Summary) Diversity() float64 {
	if s.WorstCase.Peak == 0 {
		return 0
	}

	return (s.WorstCase.Peak - s.HourAware.Peak) / s.WorstCase.Peak * 100
}

// Summarise sizes the estate three ways: as it stands, assuming every cluster
// peaks at once, and hour by hour.
func Summarise(clusters []Cluster, a Assumptions) Summary {
	s := Summary{
		Assumptions:  a,
		Clusters:     clusters,
		GrowthFactor: math.Pow(1+a.Growth/100, a.Years),
	}

	var sumOfPeaks float64

	for _, c := range clusters {
		s.Provisioned += c.Provisioned
		sumOfPeaks += c.Peak
		s.Windowed = s.Windowed || c.Windowed

		for hour := range s.Hourly {
			s.Hourly[hour] += c.VCPUAt(hour)
		}
	}

	// Found in one pass rather than by searching for the maximum value, which
	// relies on an exact float comparison to locate the hour it came from. The
	// ties are exact by construction: two hours the same clusters draw the same
	// vcpu in are summed from the same numbers in the same order, so they come
	// out bit for bit identical rather than merely close.
	peakHours := []int{0}

	for hour, total := range s.Hourly {
		switch peak := s.Hourly[peakHours[0]]; {
		case total > peak:
			peakHours = []int{hour}
		case total == peak && hour != peakHours[0]:
			peakHours = append(peakHours, hour)
		}
	}

	peakOfSums := s.Hourly[peakHours[0]]

	// The status quo is sized by what is already racked, not by demand, so it
	// carries no growth or headroom. It is the baseline the savings are
	// measured against.
	s.Current = Scenario{
		Key:      KeyCurrent,
		Label:    "Current",
		Peak:     sumOfPeaks,
		Required: s.Provisioned,
		Baseline: true,
	}

	s.WorstCase = Scenario{
		Key:   KeyWorstCase,
		Label: "Worst case",
		Peak:  sumOfPeaks,
	}

	s.HourAware = Scenario{
		Key:       KeyHourAware,
		Label:     "Hour aware",
		Peak:      peakOfSums,
		PeakHours: peakHours,
		HourAware: true,
	}

	for _, sc := range []*Scenario{&s.Current, &s.WorstCase, &s.HourAware} {
		sc.Provisioned = s.Provisioned

		if !sc.Baseline {
			sc.Required = math.Ceil(sc.Peak * s.GrowthFactor * (1 + a.Headroom/100))
		}
	}

	return s
}
