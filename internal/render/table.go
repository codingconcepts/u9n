// Package render turns a sized estate into the output formats the cli offers.
package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/codingconcepts/u9n/internal/estate"
)

// Table writes the human readable report. Each section is laid out
// independently of the others, so a long value in one doesn't pad out the
// columns of the next.
func Table(w io.Writer, s estate.Summary) error {
	sections := [][][]string{estateRows(s), scenarioRows(s)}

	var b strings.Builder

	for i, rows := range sections {
		if i > 0 {
			b.WriteString("\n")
		}

		tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		for _, row := range rows {
			// Writes are buffered, so the error surfaces from Flush below.
			_, _ = fmt.Fprintln(tw, strings.Join(row, "\t"))
		}

		if err := tw.Flush(); err != nil {
			return fmt.Errorf("laying out the table: %w", err)
		}
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("writing the table: %w", err)
	}

	return nil
}

func estateRows(s estate.Summary) [][]string {
	rows := [][]string{
		{"Clusters:", strconv.Itoa(len(s.Clusters))},
		{"Provisioned vCPU:", estate.Num(s.Provisioned)},
		{"Headroom:", estate.Num(s.Headroom) + "%"},
	}

	if s.GrowthFactor != 1 {
		rows = append(rows, []string{"Growth:", fmt.Sprintf("%s%% a year over %s years (%s in total)",
			estate.Num(s.Growth), estate.Num(s.Years), percent((s.GrowthFactor-1)*100))})
	}

	if s.Windowed {
		rows = append(rows, []string{"Peak diversity:", percent(s.Diversity())})
	}

	return rows
}

// scenarioRows lays the sizings out as columns, one row per figure, so they
// can be read across.
func scenarioRows(s estate.Summary) [][]string {
	rows := [][]string{
		{""},
		{"Required vCPU"},
		{"Peak vCPU"},
		{"Utilisation"},
		{"Peak hours"},
		{"Saved vCPU"},
	}

	for _, sc := range s.Scenarios() {
		peakHours := "-"
		if sc.HourAware {
			peakHours = estate.FormatHours(sc.PeakHours)
		}

		for i, value := range []string{
			sc.Label,
			estate.Num(sc.Required),
			estate.Num(sc.Peak),
			percent(sc.Utilisation()),
			peakHours,
			fmt.Sprintf("%s (%s)", estate.Num(sc.Saved()), percent(sc.SavedPercent())),
		} {
			rows[i] = append(rows[i], value)
		}
	}

	return rows
}

func percent(value float64) string {
	return fmt.Sprintf("%.2f%%", value)
}
