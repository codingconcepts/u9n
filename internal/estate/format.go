package estate

import (
	"fmt"
	"strconv"
	"strings"
)

// Num formats a vcpu count in full, dropping trailing zeroes and avoiding the
// scientific notation %g falls back to for large estates.
func Num(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// AllDay names a whole day's worth of hours, which is what a run of every
// hour there is reads as.
const AllDay = "all day"

// FormatHour renders an hour of the day.
func FormatHour(hour int) string {
	return fmt.Sprintf("%02d:00", hour)
}

// FormatHours renders a run of hours. A day whose every hour is the busiest
// is named rather than listed, both because the list would be the whole clock
// and because what it says is that the estate never lets up.
func FormatHours(hours []int) string {
	if len(hours) == HoursPerDay {
		return AllDay
	}

	out := make([]string, 0, len(hours))
	for _, hour := range hours {
		out = append(out, FormatHour(hour))
	}

	return strings.Join(out, ", ")
}
