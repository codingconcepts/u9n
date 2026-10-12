package estate

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

const (
	colName        = "name"
	colProvisioned = "provisioned_vcpu"
	colPeak        = "peak_vcpu"
	colBase        = "base_vcpu"
	colPeakFrom    = "peak_from"
	colPeakTo      = "peak_to"
)

// bom is the byte order mark spreadsheet software likes to put in front of a
// utf-8 csv. Left in place it becomes part of the first column's name, and the
// file is rejected for missing a column that is plainly there.
const bom = "\ufeff"

// ReadClusters reads a csv file of clusters from disk.
func ReadClusters(path string) ([]Cluster, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer func() { _ = file.Close() }()

	return ParseClusters(file)
}

// ParseClusters reads a csv of clusters. Columns may appear in any order, and
// the peak window columns are optional as long as neither or both are given.
func ParseClusters(r io.Reader) ([]Cluster, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("file is empty")
		}

		return nil, fmt.Errorf("reading header: %w", err)
	}

	index := map[string]int{}

	for i, name := range header {
		if i == 0 {
			name = strings.TrimPrefix(name, bom)
		}

		col := strings.ToLower(strings.TrimSpace(name))

		// A repeated column is ambiguous, and silently taking the last one
		// means reading a figure the file never meant to give.
		if _, ok := index[col]; ok {
			return nil, fmt.Errorf("duplicate %q column", col)
		}

		index[col] = i
	}

	for _, col := range []string{colName, colProvisioned, colPeak} {
		if _, ok := index[col]; !ok {
			return nil, fmt.Errorf("missing %q column", col)
		}
	}

	_, hasFrom := index[colPeakFrom]

	_, hasTo := index[colPeakTo]
	if hasFrom != hasTo {
		return nil, fmt.Errorf("%q and %q must be provided together", colPeakFrom, colPeakTo)
	}

	var clusters []Cluster

	seen := map[string]int{}

	for line := 2; ; line++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("reading line %d: %w", line, err)
		}

		c, err := parseCluster(record, index, hasFrom, line)
		if err != nil {
			return nil, err
		}

		if first, ok := seen[c.Name]; ok {
			return nil, fmt.Errorf("line %d: duplicate name %q, first seen on line %d", line, c.Name, first)
		}

		seen[c.Name] = line

		clusters = append(clusters, c)
	}

	if len(clusters) == 0 {
		return nil, errors.New("no clusters found")
	}

	return clusters, nil
}

func parseCluster(record []string, index map[string]int, windowed bool, line int) (Cluster, error) {
	var (
		c   Cluster
		err error
	)

	name, ok := field(record, index, colName)
	if !ok || name == "" {
		return c, fmt.Errorf("line %d: missing %s", line, colName)
	}

	c.Name = name

	if c.Provisioned, err = parseVCPU(record, index, colProvisioned, line); err != nil {
		return c, err
	}

	if c.Peak, err = parseVCPU(record, index, colPeak, line); err != nil {
		return c, err
	}

	if c.Base, err = parseVCPU(record, index, colBase, line); err != nil {
		return c, err
	}

	if c.Peak > c.Provisioned {
		return c, fmt.Errorf("line %d: %s (%s) exceeds %s (%s)", line, colPeak, Num(c.Peak), colProvisioned, Num(c.Provisioned))
	}

	if c.Base > c.Peak {
		return c, fmt.Errorf("line %d: %s (%s) exceeds %s (%s)", line, colBase, Num(c.Base), colPeak, Num(c.Peak))
	}

	if !windowed {
		return c, nil
	}

	c.Windowed = true

	// peak_to is exclusive, so 24 is allowed and means midnight.
	if c.PeakFrom, err = parseHour(record, index, colPeakFrom, line, HoursPerDay-1); err != nil {
		return c, err
	}

	if c.PeakTo, err = parseHour(record, index, colPeakTo, line, HoursPerDay); err != nil {
		return c, err
	}

	c.PeakTo %= HoursPerDay

	return c, nil
}

// parseVCPU reads a vcpu column. base_vcpu is optional and defaults to zero.
func parseVCPU(record []string, index map[string]int, col string, line int) (float64, error) {
	raw, ok := field(record, index, col)
	if !ok || raw == "" {
		if col == colBase {
			return 0, nil
		}

		return 0, fmt.Errorf("line %d: missing %s", line, col)
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("line %d: invalid %s %q", line, col, raw)
	}

	// ParseFloat accepts "NaN" and "Inf". Neither is a vcpu count, and both
	// poison every figure derived from them rather than failing loudly.
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("line %d: %s must be a finite number", line, col)
	}

	if value < 0 {
		return 0, fmt.Errorf("line %d: %s must not be negative", line, col)
	}

	return value, nil
}

func parseHour(record []string, index map[string]int, col string, line, upper int) (int, error) {
	raw, ok := field(record, index, col)
	if !ok || raw == "" {
		return 0, fmt.Errorf("line %d: missing %s", line, col)
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("line %d: invalid %s %q, expected an hour between 0 and %d", line, col, raw, upper)
	}

	if value < 0 || value > upper {
		return 0, fmt.Errorf("line %d: %s %d is out of range, expected an hour between 0 and %d", line, col, value, upper)
	}

	return value, nil
}

func field(record []string, index map[string]int, col string) (string, bool) {
	i, ok := index[col]
	if !ok || i >= len(record) {
		return "", false
	}

	return strings.TrimSpace(record[i]), true
}
