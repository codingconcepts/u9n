package main

import (
	"fmt"
	"io"

	"github.com/codingconcepts/u9n/internal/estate"
	"github.com/codingconcepts/u9n/internal/render"
	"github.com/spf13/cobra"
)

// sizeOptions is everything the size command takes.
type sizeOptions struct {
	config      string
	format      string
	assumptions estate.Assumptions
}

func (o sizeOptions) validate() error {
	// The assumptions are checked first, since a bad one makes the numbers
	// meaningless whatever they are printed as.
	if err := o.assumptions.Validate(); err != nil {
		return err
	}

	return validateFormat(o.format)
}

func sizeCommand() *cobra.Command {
	var o sizeOptions

	cmd := &cobra.Command{
		Use:   "size",
		Short: "Size the estate as it stands, at worst case, and hour by hour",
		Long: "Size the estate three ways: as it stands, assuming every cluster peaks at\n" +
			"once, and walking the 24 hour clock.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSize(cmd.OutOrStdout(), o)
		},
	}

	f := cmd.Flags()
	f.StringVar(&o.config, "config", "", "path to the csv file containing cluster vcpu data")
	f.Float64Var(&o.assumptions.Headroom, "headroom", 20, "percentage of spare capacity to add on top of peak utilisation")
	f.Float64Var(&o.assumptions.Growth, "growth", 0, "expected annual growth in vcpu demand, as a percentage")
	f.Float64Var(&o.assumptions.Years, "years", 1, "number of years to compound growth over")
	f.StringVar(&o.format, "format", formatTable, formatUsage)

	// The error only reports a flag that was never registered, which the line
	// above rules out.
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

func runSize(w io.Writer, o sizeOptions) error {
	if err := o.validate(); err != nil {
		return err
	}

	clusters, err := estate.ReadClusters(o.config)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	s := estate.Summarise(clusters, o.assumptions)

	switch o.format {
	case formatJSON:
		return render.JSON(w, s)
	case formatGraph:
		return render.Graph(w, s)
	}

	return render.Table(w, s)
}
