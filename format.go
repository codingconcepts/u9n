package main

import "fmt"

// The output formats the commands offer. Not every command draws all three,
// but the names mean the same thing wherever they are given: a report to read,
// a chart to look at, and a document to pipe.
const (
	formatTable = "table"
	formatJSON  = "json"
	formatGraph = "graph"
)

const formatUsage = "output format, one of table, json or graph"

func validateFormat(format string) error {
	switch format {
	case formatTable, formatJSON, formatGraph:
		return nil
	default:
		return fmt.Errorf("invalid format %q, expected %s, %s or %s", format, formatTable, formatJSON, formatGraph)
	}
}
