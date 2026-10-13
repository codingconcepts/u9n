package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codingconcepts/u9n/internal/estate"
	"github.com/codingconcepts/u9n/internal/render"
)

func TestProfileOptionsResolved(t *testing.T) {
	cases := []struct {
		name  string
		bands render.Bands
		exp   render.Bands
	}{
		{name: "neither asked for draws both", exp: render.Bands{Base: true, Peak: true}},
		{name: "base only", bands: render.Bands{Base: true}, exp: render.Bands{Base: true}},
		{name: "peak only", bands: render.Bands{Peak: true}, exp: render.Bands{Peak: true}},
		{
			name:  "both asked for",
			bands: render.Bands{Base: true, Peak: true},
			exp:   render.Bands{Base: true, Peak: true},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := (profileOptions{bands: c.bands}).resolved(); got != c.exp {
				t.Errorf("expected %+v, got %+v", c.exp, got)
			}
		})
	}
}

func TestProfileOptionsValidate(t *testing.T) {
	cases := []struct {
		name   string
		format string
		expErr string
	}{
		{name: "table", format: formatTable},
		{name: "json", format: formatJSON},
		{name: "graph", format: formatGraph},
		{
			name:   "unknown format",
			format: "yaml",
			expErr: `invalid format "yaml", expected table, json or graph`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := (profileOptions{format: c.format}).validate()

			switch {
			case c.expErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case c.expErr != "" && (err == nil || err.Error() != c.expErr):
				t.Fatalf("expected error %q, got %v", c.expErr, err)
			}
		})
	}
}

func TestRunProfile(t *testing.T) {
	config := testConfig(t)

	t.Run("table", func(t *testing.T) {
		var b strings.Builder

		o := profileOptions{config: config, format: formatTable}
		if err := runProfile(&b, o); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, want := range []string{"Hourly profile", "00:00", "09:00", "#"} {
			if !strings.Contains(b.String(), want) {
				t.Errorf("expected the table to contain %q, got:\n%s", want, b.String())
			}
		}
	})

	t.Run("graph draws both bands by default", func(t *testing.T) {
		var b strings.Builder

		o := profileOptions{config: config, format: formatGraph}
		if err := runProfile(&b, o); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, want := range []string{"Base", "Peak", "00", "21"} {
			if !strings.Contains(b.String(), want) {
				t.Errorf("expected the chart to contain %q, got:\n%s", want, b.String())
			}
		}
	})

	t.Run("the legend keys only the bands asked for", func(t *testing.T) {
		var b strings.Builder

		o := profileOptions{config: config, format: formatGraph, bands: render.Bands{Peak: true}}
		if err := runProfile(&b, o); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(b.String(), "Base") {
			t.Errorf("expected no base band, got:\n%s", b.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var b strings.Builder

		o := profileOptions{config: config, format: formatJSON}
		if err := runProfile(&b, o); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var out map[string]any
		if err := json.Unmarshal([]byte(b.String()), &out); err != nil {
			t.Fatalf("decoding the output: %v", err)
		}

		if hourly, ok := out["hourly"].([]any); !ok || len(hourly) != estate.HoursPerDay {
			t.Errorf("expected %d hours, got %v", estate.HoursPerDay, out["hourly"])
		}
	})

	t.Run("invalid options are caught before the file is read", func(t *testing.T) {
		var b strings.Builder

		err := runProfile(&b, profileOptions{config: "missing.csv", format: "yaml"})

		if err == nil || !strings.HasPrefix(err.Error(), "invalid format") {
			t.Fatalf("expected the format to be rejected, got %v", err)
		}
	})

	t.Run("unreadable config", func(t *testing.T) {
		var b strings.Builder

		o := profileOptions{config: filepath.Join(t.TempDir(), "missing.csv"), format: formatTable}
		err := runProfile(&b, o)

		if err == nil || !strings.HasPrefix(err.Error(), "reading config: ") {
			t.Fatalf("expected the error to say which step failed, got %v", err)
		}
	})
}
