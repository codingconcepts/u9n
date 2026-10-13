// Command u9n sizes a fleet of clusters from a csv of their vcpu footprints,
// reporting what the estate needs as it stands, if every cluster peaked at
// once, and hour by hour.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := rootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// rootCommand is the bare u9n command, which does nothing on its own beyond
// holding the subcommands that do.
func rootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "u9n",
		Short: "Calculate the utilisation of a pre and post virtualised estate",

		// Cobra prints a returned error itself and follows it with the whole
		// usage block, which would report the error twice alongside main's own
		// handling and bury it under the flag list.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddCommand(sizeCommand(), profileCommand())

	return cmd
}
