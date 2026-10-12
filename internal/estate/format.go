package estate

import (
	"fmt"
	"strconv"
)

// Num formats a vcpu count in full, dropping trailing zeroes and avoiding the
// scientific notation %g falls back to for large estates.
func Num(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// FormatHour renders an hour of the day.
func FormatHour(hour int) string {
	return fmt.Sprintf("%02d:00", hour)
}
