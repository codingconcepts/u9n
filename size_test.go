package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codingconcepts/u9n/internal/estate"
)

func TestSizeOptionsValidate(t *testing.T) {
	cases := []struct {
		name   string
		config sizeOptions
		expErr string
	}{
		{name: "defaults", config: sizeOptions{format: formatTable, assumptions: estate.Assumptions{Headroom: 20, Years: 1}}},
		{name: "json", config: sizeOptions{format: formatJSON, assumptions: estate.Assumptions{Headroom: 20, Years: 1}}},
		{name: "graph", config: sizeOptions{format: formatGraph, assumptions: estate.Assumptions{Headroom: 20, Years: 1}}},
		{
			name:   "unknown format",
			config: sizeOptions{format: "yaml", assumptions: estate.Assumptions{Years: 1}},
			expErr: `invalid format "yaml", expected table, json or graph`,
		},
		{
			// The assumptions are checked first, since a bad one makes the
			// numbers meaningless whatever they are printed as.
			name:   "bad assumptions",
			config: sizeOptions{format: formatTable, assumptions: estate.Assumptions{Headroom: -1}},
			expErr: "headroom must not be negative",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.config.validate()

			switch {
			case c.expErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case c.expErr != "" && (err == nil || err.Error() != c.expErr):
				t.Fatalf("expected error %q, got %v", c.expErr, err)
			}
		})
	}
}

func TestRunSize(t *testing.T) {
	config := testConfig(t)

	assumptions := estate.Assumptions{Headroom: 20, Years: 1}

	t.Run("table", func(t *testing.T) {
		var b strings.Builder
		if err := runSize(&b, sizeOptions{config: config, format: formatTable, assumptions: assumptions}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, want := range []string{"Clusters:", "Required vCPU", "Hour aware"} {
			if !strings.Contains(b.String(), want) {
				t.Errorf("expected the output to contain %q, got:\n%s", want, b.String())
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		var b strings.Builder
		if err := runSize(&b, sizeOptions{config: config, format: formatJSON, assumptions: assumptions}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var out map[string]any
		if err := json.Unmarshal([]byte(b.String()), &out); err != nil {
			t.Fatalf("decoding the output: %v", err)
		}

		if out["clusters"] != 1.0 {
			t.Errorf("expected 1 cluster, got %v", out["clusters"])
		}
	})

	t.Run("invalid options are caught before the file is read", func(t *testing.T) {
		var b strings.Builder

		err := runSize(&b, sizeOptions{config: "missing.csv", format: "yaml", assumptions: assumptions})

		if err == nil || !strings.HasPrefix(err.Error(), "invalid format") {
			t.Fatalf("expected the format to be rejected, got %v", err)
		}
	})

	t.Run("unreadable config", func(t *testing.T) {
		var b strings.Builder

		err := runSize(&b, sizeOptions{config: filepath.Join(t.TempDir(), "missing.csv"), format: formatTable, assumptions: assumptions})

		if err == nil || !strings.HasPrefix(err.Error(), "reading config: ") {
			t.Fatalf("expected the error to say which step failed, got %v", err)
		}
	})
}
