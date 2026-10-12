// Command u9n sizes a fleet of clusters from a csv of their vcpu footprints,
// reporting what the estate needs as it stands, if every cluster peaked at
// once, and hour by hour.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/codingconcepts/u9n/internal/estate"
	"github.com/codingconcepts/u9n/internal/render"
)

const (
	formatTable = "table"
	formatJSON  = "json"
	formatGraph = "graph"
)

// options is everything the cli takes.
type options struct {
	config      string
	format      string
	profile     bool
	assumptions estate.Assumptions
}

func (o options) validate() error {
	if err := o.assumptions.Validate(); err != nil {
		return err
	}

	switch o.format {
	case formatTable, formatJSON, formatGraph:
		return nil
	default:
		return fmt.Errorf("invalid format %q, expected %s, %s or %s", o.format, formatTable, formatJSON, formatGraph)
	}
}

func main() {
	var o options

	flag.StringVar(&o.config, "config", "", "path to the csv file containing cluster vcpu data")
	flag.Float64Var(&o.assumptions.Headroom, "headroom", 20, "percentage of spare capacity to add on top of peak utilisation")
	flag.Float64Var(&o.assumptions.Growth, "growth", 0, "expected annual growth in vcpu demand, as a percentage")
	flag.Float64Var(&o.assumptions.Years, "years", 1, "number of years to compound growth over")
	flag.BoolVar(&o.profile, "profile", false, "print the hourly vcpu profile of the consolidated estate")
	flag.StringVar(&o.format, "format", formatTable, "output format, one of table, json or graph")
	flag.Parse()

	if o.config == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(os.Stdout, o); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(w io.Writer, o options) error {
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

	return render.Table(w, s, o.profile)
}
