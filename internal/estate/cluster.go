// Package estate models how much vcpu a fleet of clusters needs across a
// representative day, and the sizings that could serve it.
package estate

// HoursPerDay is the length of the clock everything is modelled over. The
// model is one representative day, so a single 24 hour profile is all it keeps.
const HoursPerDay = 24

// Cluster is one cluster's vcpu footprint: what it has, what it draws at peak
// and off peak, and when its peak window falls.
type Cluster struct {
	Name        string
	Provisioned float64
	Peak        float64
	Base        float64
	PeakFrom    int
	PeakTo      int
	Windowed    bool
}

// InPeak reports whether the cluster is at peak during the given hour. The
// window is half-open, so a PeakFrom of 9 and a PeakTo of 17 covers 09:00
// until 16:59. Windows that wrap midnight are supported, and a window whose
// bounds are equal covers the whole day.
func (c Cluster) InPeak(hour int) bool {
	switch {
	case !c.Windowed, c.PeakFrom == c.PeakTo:
		return true
	case c.PeakFrom < c.PeakTo:
		return hour >= c.PeakFrom && hour < c.PeakTo
	default:
		return hour >= c.PeakFrom || hour < c.PeakTo
	}
}

// VCPUAt is the vcpu the cluster draws in the given hour.
func (c Cluster) VCPUAt(hour int) float64 {
	if c.InPeak(hour) {
		return c.Peak
	}

	return c.Base
}

// Demand is one hour's draw, split into the part the estate holds around the
// clock and the part its peak windows add on top.
type Demand struct {
	Base   float64
	Uplift float64
}

// Total is the whole draw for the hour.
func (d Demand) Total() float64 {
	return d.Base + d.Uplift
}

// HourlyDemand splits every hour of the day into base and peak uplift. Each
// cluster draws its base around the clock, and the clusters inside their peak
// window add the difference between peak and base on top, so the two parts sum
// to what VCPUAt reports. Parsing rejects a base above its own peak, so the
// uplift is never negative.
//
// Without peak windows every cluster counts as permanently at peak, so the
// uplift is the same in all 24 hours and the split says only how much of the
// draw is the floor.
func HourlyDemand(clusters []Cluster) [HoursPerDay]Demand {
	var out [HoursPerDay]Demand

	for _, c := range clusters {
		for hour := range out {
			out[hour].Base += c.Base

			if c.InPeak(hour) {
				out[hour].Uplift += c.Peak - c.Base
			}
		}
	}

	return out
}
