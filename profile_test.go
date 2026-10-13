package main

import (
	"path/filepath"
	"strings"
	"testing"

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

func TestRunProfile(t *testing.T) {
	config := testConfig(t)

	t.Run("draws both bands by default", func(t *testing.T) {
		var b strings.Builder
		if err := runProfile(&b, profileOptions{config: config}); err != nil {
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

		o := profileOptions{config: config, bands: render.Bands{Peak: true}}
		if err := runProfile(&b, o); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(b.String(), "Base") {
			t.Errorf("expected no base band, got:\n%s", b.String())
		}
	})

	t.Run("unreadable config", func(t *testing.T) {
		var b strings.Builder

		err := runProfile(&b, profileOptions{config: filepath.Join(t.TempDir(), "missing.csv")})

		if err == nil || !strings.HasPrefix(err.Error(), "reading config: ") {
			t.Fatalf("expected the error to say which step failed, got %v", err)
		}
	})
}
