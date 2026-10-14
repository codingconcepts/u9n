package main

import (
	"fmt"
	"io"

	"github.com/codingconcepts/u9n/internal/estate"
	"github.com/codingconcepts/u9n/internal/render"
	"github.com/codingconcepts/u9n/internal/text"
	"github.com/spf13/cobra"
)

// compareOptions is everything the compare command takes.
type compareOptions struct {
	config string
}

func compareCommand() *cobra.Command {
	var o compareOptions

	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Chart every cluster on hardware of its own, and the total that comes to",
		Long: text.Wrap("Chart the estate the way it is racked when every cluster gets hardware of its\n"+
			"own: a cluster at a time, each drawn from the base it holds all day up to the\n"+
			"peak its own hardware has to cover.\n\n"+
			"Every bar is drawn to one scale, so the total underneath them is those same\n"+
			"bars laid end to end: the sum of the peaks, which is what separate hardware\n"+
			"has to stand up because no cluster can lend its idle capacity to another.\n\n"+
			"Under it is the same total taken at the hour the clusters are busiest\n"+
			"together, each segment cut back to what that cluster draws in the hour, which\n"+
			"is the sizing shared hardware needs. The length the bar loses is the capacity\n"+
			"only separate hardware pays for.\n\n"+
			"This compares demand rather than sizings, so it carries no headroom or growth.\n"+
			"Use the size command for what the estate has to rack, and the peak command for\n"+
			"the same gap read across the whole day.", text.Width),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCompare(cmd.OutOrStdout(), o)
		},
	}

	f := cmd.Flags()
	f.StringVar(&o.config, "config", "", "path to the csv file containing cluster vcpu data")

	// The error only reports a flag that was never registered, which the line
	// above rules out.
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

func runCompare(w io.Writer, o compareOptions) error {
	clusters, err := estate.ReadClusters(o.config)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	// Summarised without headroom or growth, which the chart doesn't draw. A
	// year of no growth leaves the peaks as the config reports them.
	return render.Compare(w, estate.Summarise(clusters, estate.Assumptions{Years: 1}))
}
