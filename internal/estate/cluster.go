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
