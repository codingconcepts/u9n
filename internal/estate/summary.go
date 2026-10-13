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

// The two ends of the day the hours can be read off: the hours the estate is
// busiest in, which the sizings are taken from, and the hours it is quietest
// in, which is where a sizing for the busiest wastes most.
const (
	busiest  = true
	quietest = false
)

// tiedHours are the hours holding one end of the day, in order. They are found
// in one pass rather than by searching for a value, which would rely on an
// exact float comparison to locate the hour it came from. The ties are exact
// by construction: two hours the same clusters draw the same vcpu in are
// summed from the same numbers in the same order, so they come out bit for bit
// identical rather than merely close.
func (s Summary) tiedHours(wantBusiest bool) []int {
	found := []int{0}

	for hour, total := range s.Hourly {
		switch best := s.Hourly[found[0]]; {
		case total > best == wantBusiest && total != best:
			found = []int{hour}
		case total == best && hour != found[0]:
			found = append(found, hour)
		}
	}

	return found
}

// QuietHours are the hours the estate draws least in, in order. They are the
// hours a sizing for the busiest hour holds the most capacity nothing is
// drawing on.
func (s Summary) QuietHours() []int {
	return s.tiedHours(quietest)
}

// VCPUHours is what the estate draws across the whole day rather than in the
// hour it is busiest, which is the figure a rack is actually paid for.
func (s Summary) VCPUHours() float64 {
	var total float64
	for _, hour := range s.Hourly {
		total += hour
	}

	return total
}

// WorstCaseVCPUHours is what sizing for the sum of the peaks stands up over
// the same day, which is the worst case held for all 24 hours whether or not
// anything is drawing on it.
func (s Summary) WorstCaseVCPUHours() float64 {
	return s.WorstCase.Peak * HoursPerDay
}

// IdleVCPUHours is the capacity the worst case racks and nothing draws on. It
// is the day long version of the peak diversity, and the larger number: the
// diversity is measured in the one hour the estate is busiest, which is the
// hour the worst case comes closest to being justified in.
func (s Summary) IdleVCPUHours() float64 {
	return s.WorstCaseVCPUHours() - s.VCPUHours()
}

// IdlePercent is IdleVCPUHours as a share of what the worst case racks.
func (s Summary) IdlePercent() float64 {
	if worstCase := s.WorstCaseVCPUHours(); worstCase != 0 {
		return s.IdleVCPUHours() / worstCase * 100
	}

	return 0
}

// MeanUtilisation is how hard a worst case sizing runs across the day, as
// against Scenario.Utilisation, which measures it in its busiest hour.
func (s Summary) MeanUtilisation() float64 {
	if worstCase := s.WorstCaseVCPUHours(); worstCase != 0 {
		return s.VCPUHours() / worstCase * 100
	}

	return 0
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

	peakHours := s.tiedHours(busiest)
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
