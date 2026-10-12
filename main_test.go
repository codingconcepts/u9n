package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codingconcepts/u9n/internal/estate"
)

func TestOptionsValidate(t *testing.T) {
	cases := []struct {
		name   string
		config options
		expErr string
	}{
		{name: "defaults", config: options{format: formatTable, assumptions: estate.Assumptions{Headroom: 20, Years: 1}}},
		{name: "json", config: options{format: formatJSON, assumptions: estate.Assumptions{Headroom: 20, Years: 1}}},
		{name: "graph", config: options{format: formatGraph, assumptions: estate.Assumptions{Headroom: 20, Years: 1}}},
		{
			name:   "unknown format",
			config: options{format: "yaml", assumptions: estate.Assumptions{Years: 1}},
			expErr: `invalid format "yaml", expected table, json or graph`,
		},
		{
			// The assumptions are checked first, since a bad one makes the
			// numbers meaningless whatever they are printed as.
			name:   "bad assumptions",
			config: options{format: formatTable, assumptions: estate.Assumptions{Headroom: -1}},
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

func TestRun(t *testing.T) {
	config := filepath.Join(t.TempDir(), "clusters.csv")

	csv := "name,provisioned_vcpu,peak_vcpu,base_vcpu,peak_from,peak_to\na,10,5,1,9,11\n"
	if err := os.WriteFile(config, []byte(csv), 0o600); err != nil {
		t.Fatalf("writing the config: %v", err)
	}

	assumptions := estate.Assumptions{Headroom: 20, Years: 1}

	t.Run("table", func(t *testing.T) {
		var b strings.Builder
		if err := run(&b, options{config: config, format: formatTable, assumptions: assumptions}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, want := range []string{"Clusters:", "Required vCPU", "Hour aware"} {
			if !strings.Contains(b.String(), want) {
				t.Errorf("expected the output to contain %q, got:\n%s", want, b.String())
			}
		}

		if strings.Contains(b.String(), "Hourly profile") {
			t.Error("expected no hourly profile unless it is asked for")
		}
	})

	t.Run("table with the profile", func(t *testing.T) {
		var b strings.Builder
		if err := run(&b, options{config: config, format: formatTable, profile: true, assumptions: assumptions}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(b.String(), "Hourly profile") {
			t.Errorf("expected an hourly profile, got:\n%s", b.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var b strings.Builder
		if err := run(&b, options{config: config, format: formatJSON, assumptions: assumptions}); err != nil {
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

		err := run(&b, options{config: "missing.csv", format: "yaml", assumptions: assumptions})

		if err == nil || !strings.HasPrefix(err.Error(), "invalid format") {
			t.Fatalf("expected the format to be rejected, got %v", err)
		}
	})

	t.Run("unreadable config", func(t *testing.T) {
		var b strings.Builder

		err := run(&b, options{config: filepath.Join(t.TempDir(), "missing.csv"), format: formatTable, assumptions: assumptions})

		if err == nil || !strings.HasPrefix(err.Error(), "reading config: ") {
			t.Fatalf("expected the error to say which step failed, got %v", err)
		}
	})
}
