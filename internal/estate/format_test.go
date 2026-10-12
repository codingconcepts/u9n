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
