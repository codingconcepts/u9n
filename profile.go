package main

import (
	"fmt"
	"io"

	"github.com/codingconcepts/u9n/internal/estate"
	"github.com/codingconcepts/u9n/internal/render"
	"github.com/spf13/cobra"
)

// profileOptions is everything the profile command takes.
type profileOptions struct {
	config string
	format string
	bands  render.Bands
}

func (o profileOptions) validate() error {
	return validateFormat(o.format)
}

// resolved is the bands to draw. Asking for neither draws both, since the one
// thing nobody means by it is an empty chart.
func (o profileOptions) resolved() render.Bands {
	if !o.bands.Base && !o.bands.Peak {
		return render.Bands{Base: true, Peak: true}
	}

	return o.bands
}

func profileCommand() *cobra.Command {
	var o profileOptions

	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Report what the estate draws each hour from 00:00 to 23:00",
		Long: "Report what the estate draws each hour, as a row an hour, as a stacked bar a\n" +
			"day wide, or as json. The base band is the vcpu every cluster holds around\n" +
			"the clock, and the peak band what the peak windows add on top of it.\n\n" +
			"This is today's demand rather than a sizing, so it carries no headroom or\n" +
			"growth. Use the size command for those.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProfile(cmd.OutOrStdout(), o)
		},
	}

	f := cmd.Flags()
	f.StringVar(&o.config, "config", "", "path to the csv file containing cluster vcpu data")
	f.BoolVar(&o.bands.Base, "base", false, "show the vcpu the estate holds around the clock")
	f.BoolVar(&o.bands.Peak, "peak", false, "show the vcpu the peak windows add on top of the base")
	f.StringVar(&o.format, "format", formatTable, formatUsage)

	// The error only reports a flag that was never registered, which the line
	// above rules out.
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

func runProfile(w io.Writer, o profileOptions) error {
	if err := o.validate(); err != nil {
		return err
	}

	clusters, err := estate.ReadClusters(o.config)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	bands := o.resolved()

	switch o.format {
	case formatJSON:
		return render.ProfileJSON(w, clusters, bands)
	case formatGraph:
		return render.ProfileGraph(w, clusters, bands)
	}

	return render.ProfileTable(w, clusters, bands)
}
