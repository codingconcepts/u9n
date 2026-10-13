package estate

import (
	"math"
	"testing"
)

// Num must never fall back to scientific notation, which would silently
// misreport a large estate by orders of magnitude.
func TestNum(t *testing.T) {
	cases := map[float64]string{
		0:         "0",
		6:         "6",
		20.5:      "20.5",
		1000000:   "1000000",
		1440000:   "1440000",
		-4:        "-4",
		math.Pi:   "3.141592653589793",
		0.0000001: "0.0000001",
	}

	for in, exp := range cases {
		if act := Num(in); act != exp {
			t.Errorf("Num(%v): expected %q, got %q", in, exp, act)
		}
	}
}

func TestFormatHour(t *testing.T) {
	cases := map[int]string{0: "00:00", 9: "09:00", 23: "23:00"}

	for in, exp := range cases {
		if act := FormatHour(in); act != exp {
			t.Errorf("FormatHour(%d): expected %q, got %q", in, exp, act)
		}
	}
}

func TestFormatHours(t *testing.T) {
	allDay := make([]int, 0, HoursPerDay)
	for hour := range HoursPerDay {
		allDay = append(allDay, hour)
	}

	cases := []struct {
		name  string
		hours []int
		exp   string
	}{
		{name: "one hour", hours: []int{20}, exp: "20:00"},
		{name: "a run of hours", hours: []int{20, 21, 22}, exp: "20:00, 21:00, 22:00"},
		{name: "hours apart", hours: []int{2, 18}, exp: "02:00, 18:00"},
		{name: "a day that never lets up", hours: allDay, exp: "all day"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if act := FormatHours(c.hours); act != c.exp {
				t.Errorf("FormatHours(%v): expected %q, got %q", c.hours, c.exp, act)
			}
		})
	}
}
