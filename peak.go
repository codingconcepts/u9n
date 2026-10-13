package main

import (
	"fmt"
	"io"

	"github.com/codingconcepts/u9n/internal/estate"
	"github.com/codingconcepts/u9n/internal/render"
	"github.com/spf13/cobra"
)

// peakOptions is everything the peak command takes.
type peakOptions struct {
	config string
}

func peakCommand() *cobra.Command {
	var o peakOptions

	cmd := &cobra.Command{
		Use:   "peak",
		Short: "Chart the sum of the peaks against the peak of the sum",
		Long: "Chart the two numbers an estate can be sized for: the sum of every cluster's\n" +
			"peak, which assumes they all peak at the same instant, and the peak of what\n" +
			"they draw together, which is the busiest hour of the day they actually share.\n\n" +
			"The gap between the two is the capacity bought for peaks that never land in\n" +
			"the same hour. Each cluster is then drawn as its own peak, with the part of it\n" +
			"the estate is serving at the peak hour filled in and the rest shaded, so the\n" +
			"clusters driving the gap can be picked out by eye.\n\n" +
			"This compares demand rather than sizings, so it carries no headroom or growth.\n" +
			"Both numbers scale by the same factor under either, which moves them without\n" +
			"moving the gap. Use the size command for what the estate has to rack.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPeak(cmd.OutOrStdout(), o)
		},
	}

	f := cmd.Flags()
	f.StringVar(&o.config, "config", "", "path to the csv file containing cluster vcpu data")

	// The error only reports a flag that was never registered, which the line
	// above rules out.
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

func runPeak(w io.Writer, o peakOptions) error {
	clusters, err := estate.ReadClusters(o.config)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	// Summarised without headroom or growth, which the chart doesn't draw. A
	// year of no growth leaves the peaks as the config reports them.
	return render.Peak(w, estate.Summarise(clusters, estate.Assumptions{Years: 1}))
}
