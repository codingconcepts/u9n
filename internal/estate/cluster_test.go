package estate

import "testing"

func TestInPeak(t *testing.T) {
	cases := []struct {
		name    string
		cluster Cluster
		exp     []int
	}{
		{
			name:    "no window covers every hour",
			cluster: Cluster{},
			exp:     hours(0, 24),
		},
		{
			name:    "daytime window excludes its upper bound",
			cluster: Cluster{Windowed: true, PeakFrom: 9, PeakTo: 17},
			exp:     hours(9, 17),
		},
		{
			name:    "window wrapping midnight",
			cluster: Cluster{Windowed: true, PeakFrom: 22, PeakTo: 3},
			exp:     append(hours(22, 24), hours(0, 3)...),
		},
		{
			name:    "equal bounds cover every hour",
			cluster: Cluster{Windowed: true, PeakFrom: 6, PeakTo: 6},
			exp:     hours(0, 24),
		},
		{
			name:    "single hour window",
			cluster: Cluster{Windowed: true, PeakFrom: 13, PeakTo: 14},
			exp:     hours(13, 14),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			expected := map[int]bool{}
			for _, hour := range c.exp {
				expected[hour] = true
			}

			for hour := range HoursPerDay {
				if act := c.cluster.InPeak(hour); act != expected[hour] {
					t.Fatalf("hour %d: expected InPeak %v, got %v", hour, expected[hour], act)
				}
			}
		})
	}
}

// Off peak hours draw the base load, which is zero for a cluster that doesn't
// declare one.
func TestVCPUAt(t *testing.T) {
	cases := []struct {
		name    string
		cluster Cluster
		exp     map[int]float64
	}{
		{
			name:    "windowed",
			cluster: Cluster{Peak: 10, Base: 2, PeakFrom: 9, PeakTo: 11, Windowed: true},
			exp:     map[int]float64{8: 2, 9: 10, 10: 10, 11: 2},
		},
		{
			name:    "without a window the peak is drawn all day",
			cluster: Cluster{Peak: 10, Base: 2},
			exp:     map[int]float64{0: 10, 12: 10, 23: 10},
		},
		{
			name:    "without a base load the estate idles at zero",
			cluster: Cluster{Peak: 10, PeakFrom: 9, PeakTo: 10, Windowed: true},
			exp:     map[int]float64{8: 0, 9: 10, 10: 0},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for hour, exp := range c.exp {
				if act := c.cluster.VCPUAt(hour); act != exp {
					t.Errorf("hour %d: expected %g, got %g", hour, exp, act)
				}
			}
		})
	}
}

func hours(from, to int) []int {
	out := make([]int, 0, to-from)
	for hour := from; hour < to; hour++ {
		out = append(out, hour)
	}

	return out
}
